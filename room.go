package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Room maintains the set of active clients and broadcasts messages to the clients.
type Room struct {
	ID string
	lobby *Lobby
	
	clients map[*Client]string

	inputs chan struct {
		client *Client
		input  ClientInput
	}

	register chan *Client
	unregister chan *Client

	gameState   *GameState
	config      GameSettings
	projCounter int
	
	stateMutex sync.Mutex
}

func newRoom(id string, lobby *Lobby) *Room {
	r := &Room{
		ID: id,
		lobby: lobby,
		inputs: make(chan struct {
			client *Client
			input  ClientInput
		}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]string),
		config:     DefaultConfig,
	}
	r.initGame()
	return r
}

// initGame completely resets the room's game state
func (r *Room) initGame() {
	r.stateMutex.Lock()
	defer r.stateMutex.Unlock()

	var existingPlayers map[string]*PlayerState
	if r.gameState != nil {
		existingPlayers = r.gameState.Players
	} else {
		existingPlayers = make(map[string]*PlayerState)
	}

	r.gameState = &GameState{
		Status:      "waiting",
		Players:     existingPlayers,
		Projectiles: make([]Projectile, 0),
		GameOver:    false,
		Winner:      "",
	}
	r.projCounter = 0

	// If we have existing players, reset their positions, health, and cooldowns
	if len(existingPlayers) > 0 {
		var pids []string
		for pid := range existingPlayers {
			pids = append(pids, pid)
		}
		sort.Strings(pids)

		blueCount, redCount := 0, 0
		for _, pid := range pids {
			p := existingPlayers[pid]
			
			startX := r.config.BoardWidth * 0.25
			var teamCount int
			if p.Team == "red" {
				startX = r.config.BoardWidth * 0.75
				teamCount = redCount
				redCount++
			} else {
				teamCount = blueCount
				blueCount++
			}
			startY := (r.config.BoardHeight / 2) + float64(teamCount*80 - 80) // Staggering

			p.X = startX
			p.Y = startY
			p.HP = r.config.MaxHP
			p.MaxHP = r.config.MaxHP
			p.Radius = r.config.PlayerRadius
			
			// Reset all cooldowns
			p.IsCharging = false
			p.ChargeTime = 0
			p.MaxCharge = r.config.MaxChargeTime
			p.IsDashing = false
			p.DashTimer = 0
			p.DashCooldown = 0
			p.IsReflecting = false
			p.ReflectTimer = 0
			p.ReflectCooldown = 0
			p.AttackCooldown = 0
			p.DashDir = Vector{X: 0, Y: 0}
			p.HasDamageBuff = false
		}
	}
}

// Run starts the room's message pump and game loop.
func (r *Room) Run() {
	ticker := time.NewTicker(time.Millisecond * 16) // ~60 FPS
	defer ticker.Stop()

	for {
		select {
		case client := <-r.register:
			r.stateMutex.Lock()
			
			// Count teams
			blueCount := 0
			redCount := 0
			for _, role := range r.clients {
				if r.gameState.Players[role] != nil {
					if r.gameState.Players[role].Team == "blue" {
						blueCount++
					} else if r.gameState.Players[role].Team == "red" {
						redCount++
					}
				}
			}

			role := "spectator"
			
			if blueCount < r.config.MaxPlayersPerTeam || redCount < r.config.MaxPlayersPerTeam {
				// Find an available ID (p1 through p10)
				for i := 1; i <= r.config.MaxPlayersPerTeam*2; i++ {
					pid := fmt.Sprintf("p%d", i)
					if r.gameState.Players[pid] == nil {
						role = pid
						break
					}
				}
				
				if role != "spectator" {
					// Assign team (balance if possible)
					team := "blue"
					if blueCount > redCount {
						team = "red"
					}
					
					startX := r.config.BoardWidth * 0.25
					color := "#3498db" // blue
					if team == "red" {
						startX = r.config.BoardWidth * 0.75
						color = "#e74c3c" // red
					}
					
					startY := (r.config.BoardHeight / 2) + float64((redCount+blueCount)*40 - 80)
					
					r.gameState.Players[role] = &PlayerState{
						ID:        role,
						Team:      team,
						X:         startX,
						Y:         startY,
						Radius:    r.config.PlayerRadius,
						Color:     color,
						HP:        r.config.MaxHP,
						MaxHP:     r.config.MaxHP,
						MaxCharge: r.config.MaxChargeTime,
					}
				}
			}
			r.stateMutex.Unlock()

			r.clients[client] = role
			client.role = role

			// Send init message
			initMsg := InitMessage{
				Type:     "init",
				Role:     role,
				Width:    r.config.BoardWidth,
				Height:   r.config.BoardHeight,
				Settings: r.config,
			}
			
			// Marshal to byte slice to send via channel
			msgBytes, _ := json.Marshal(initMsg)
			client.send <- msgBytes

		case client := <-r.unregister:
			if role, ok := r.clients[client]; ok {
				delete(r.clients, client)

				r.stateMutex.Lock()
				delete(r.gameState.Players, role)
				r.stateMutex.Unlock()
				
				// Tell lobby we exited, if we didn't just DC completely
				if r.lobby != nil {
					// We do not close(client.send) here because they return to Lobby!
					r.lobby.clients[client] = true
					r.lobby.sendRoomListTo(client)
				}
				
				// If room is empty, close it
				if len(r.clients) == 0 {
					if r.lobby != nil {
						r.lobby.closeRoom <- r.ID
					}
					return // End the goroutine
				}
			}

		case msg := <-r.inputs:
			input := msg.input
			client := msg.client
			role := client.role

			if input.Type == "START_GAME" && r.gameState.Status == "waiting" {
				r.stateMutex.Lock()
				if len(r.gameState.Players) >= 2 {
					r.gameState.Status = "playing"
				}
				r.stateMutex.Unlock()
				continue
			}

			if input.Type == "restart" {
				r.initGame()
				continue
			}

			if input.Type == "update_settings" && input.Settings != nil {
				r.stateMutex.Lock()
				r.config = *input.Settings
				for _, p := range r.gameState.Players {
					p.Radius = r.config.PlayerRadius
					p.MaxCharge = r.config.MaxChargeTime
					p.MaxHP = r.config.MaxHP
					if p.HP > p.MaxHP {
						p.HP = p.MaxHP
					}
				}
				r.stateMutex.Unlock()
				continue
			}

			if role != "spectator" {
				r.processInput(role, input)
			}

		case <-ticker.C:
			r.updateState()
			r.broadcastState()
		}
	}
}

func (r *Room) processInput(role string, input ClientInput) {
	r.stateMutex.Lock()
	defer r.stateMutex.Unlock()

	p := r.gameState.Players[role]
	if p == nil || r.gameState.GameOver {
		return
	}

	dt := 1.0 / 60.0

	// Cooldown
	if p.DashCooldown > 0 {
		p.DashCooldown -= dt
	}
	if p.ReflectCooldown > 0 {
		p.ReflectCooldown -= dt
	}
	if p.AttackCooldown > 0 {
		p.AttackCooldown -= dt
	}

	// Reflect Trigger (only if not dashing and charging)
	if input.Reflect && p.ReflectCooldown <= 0 && !p.IsReflecting && !p.IsDashing && !p.IsCharging {
		p.IsReflecting = true
		p.ReflectTimer = 0.3 // 0.3s duration
		p.ReflectCooldown = r.config.ReflectCooldownLimit
	}
	if p.IsReflecting {
		p.ReflectTimer -= dt
		if p.ReflectTimer <= 0 {
			p.IsReflecting = false
		}
	}

	// Movement vector
	var dx, dy float64
	if input.Up {
		dy -= 1
	}
	if input.Down {
		dy += 1
	}
	if input.Left {
		dx -= 1
	}
	if input.Right {
		dx += 1
	}

	if dx != 0 || dy != 0 {
		len := math.Hypot(dx, dy)
		dx /= len
		dy /= len
	}

	// Dash Trigger
	if input.Dash && p.DashCooldown <= 0 && !p.IsDashing && !p.IsReflecting {
		p.IsDashing = true
		p.DashTimer = 0.2 // 0.2s duration fixed
		p.DashCooldown = r.config.DashCooldownLimit

		if dx == 0 && dy == 0 {
			// Dash towards mouse if not moving
			mx := input.MouseX - p.X
			my := input.MouseY - p.Y
			mlen := math.Hypot(mx, my)
			if mlen > 0 {
				p.DashDir = Vector{X: mx / mlen, Y: my / mlen}
			} else {
				if p.Team == "blue" {
					p.DashDir = Vector{X: 1, Y: 0}
				} else {
					p.DashDir = Vector{X: -1, Y: 0}
				}
			}
		} else {
			p.DashDir = Vector{X: dx, Y: dy}
		}
		p.IsCharging = false
		p.ChargeTime = 0
	}

	// Charging / Shooting
	if p.IsDashing {
		p.DashTimer -= dt
		if p.DashTimer <= 0 {
			p.IsDashing = false
		}
	} else {
		if input.IsAttack && p.AttackCooldown <= 0 && r.gameState.Status == "playing" {
			p.IsCharging = true
			p.ChargeTime += dt
			if p.ChargeTime > r.config.MaxChargeTime {
				p.ChargeTime = r.config.MaxChargeTime
			}
		} else if p.IsCharging {
			// Release -> Shoot
			if p.ChargeTime >= r.config.MinChargeTime && r.gameState.Status == "playing" {
				r.shoot(role, p, input.MouseX, input.MouseY)
				p.AttackCooldown = r.config.AttackCooldownLimit
			}
			p.IsCharging = false
			p.ChargeTime = 0
		}
	}

	// Apply movement
	currentSpeed := r.config.BaseSpeed
	if p.IsDashing {
		currentSpeed = r.config.DashDistance / 0.2 // Since DashTimer is 0.2
		dx = p.DashDir.X
		dy = p.DashDir.Y
	} else if p.IsCharging {
		currentSpeed = r.config.BaseSpeed * r.config.ChargeSpeedMult
	} else if p.IsReflecting {
		currentSpeed = r.config.BaseSpeed * 0.3 // Slowed slightly while reflecting
	}

	p.X += dx * currentSpeed * dt
	p.Y += dy * currentSpeed * dt

	// Bounds checking
	p.Y = math.Max(p.Radius, math.Min(r.config.BoardHeight-p.Radius, p.Y))
	if p.Team == "blue" {
		p.X = math.Max(p.Radius, math.Min(r.config.BoardWidth/2-p.Radius, p.X))
	} else {
		p.X = math.Max(r.config.BoardWidth/2+p.Radius, math.Min(r.config.BoardWidth-p.Radius, p.X))
	}
}

func (r *Room) shoot(role string, p *PlayerState, mx float64, my float64) {
	dx := mx - p.X
	dy := my - p.Y
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		dist = 1
	}

	damageMult := 1.0
	if p.HasDamageBuff {
		damageMult = 1.5
		p.HasDamageBuff = false
	}

	isSuper := p.ChargeTime >= r.config.MaxChargeTime
	pSpeed := r.config.MissileSpeed
	pRadius := r.config.MissileRadius
	pColor := p.Color

	if isSuper {
		pSpeed = r.config.SuperMissileSpeed
		pRadius = r.config.SuperMissileRadius
		pColor = "#f1c40f"
	}

	r.projCounter++
	r.gameState.Projectiles = append(r.gameState.Projectiles, Projectile{
		ID:         r.projCounter,
		OwnerID:    role,
		TeamID:     p.Team,
		X:          p.X,
		Y:          p.Y,
		VX:         (dx / dist) * pSpeed,
		VY:         (dy / dist) * pSpeed,
		Radius:     pRadius,
		Color:      pColor,
		IsSuper:    isSuper,
		DamageMult: damageMult,
	})
}

func (r *Room) updateState() {
	r.stateMutex.Lock()
	defer r.stateMutex.Unlock()

	if r.gameState.GameOver {
		return
	}

	dt := 1.0 / 60.0

	// Team survival check
	redAlive := false
	blueAlive := false
	activePlayers := 0

	for _, p := range r.gameState.Players {
		if p.HP > 0 {
			activePlayers++
			if p.Team == "red" {
				redAlive = true
			} else if p.Team == "blue" {
				blueAlive = true
			}
		}
	}

	if activePlayers > 0 && r.gameState.Status == "playing" {
		if !redAlive && blueAlive {
			r.gameState.GameOver = true
			r.gameState.Winner = "🔵 Blue 팀 승리!"
			r.gameState.Status = "finished"
			return
		} else if !blueAlive && redAlive {
			r.gameState.GameOver = true
			r.gameState.Winner = "🔴 Red 팀 승리!"
			r.gameState.Status = "finished"
			return
		} else if !blueAlive && !redAlive {
			// Shouldn't really happen but edge case where both die frame perfect
			r.gameState.GameOver = true
			r.gameState.Winner = "무승부!"
			r.gameState.Status = "finished"
			return
		}
	}

	// Projectiles logic
	aliveProjectiles := make([]Projectile, 0)

	for _, p := range r.gameState.Projectiles {
		p.X += p.VX * dt
		p.Y += p.VY * dt

		// Map OOB
		if p.X < 0 || p.X > r.config.BoardWidth || p.Y < 0 || p.Y > r.config.BoardHeight {
			continue
		}

		hit := false
		for targetID, target := range r.gameState.Players {
			if targetID == p.OwnerID {
				continue // don't hit self
			}

			if target.Team == p.TeamID {
				continue // Friendly fire disabled
			}

			if target.IsReflecting {
				dist := math.Hypot(p.X-target.X, p.Y-target.Y)
				if dist < p.Radius+target.Radius+10 { // Reflection shield radius padding
					// 저스트 디펜스 버프 획득
					target.HasDamageBuff = true

					// Reflect the projectile towards the original owner
					originalOwnerID := p.OwnerID
					originalOwner := r.gameState.Players[originalOwnerID]

					var dirX, dirY float64
					if originalOwner != nil {
						dirX = originalOwner.X - target.X
						dirY = originalOwner.Y - target.Y
					} else {
						dirX = p.VX * -1
						dirY = p.VY * -1
					}

					dirDist := math.Hypot(dirX, dirY)
					if dirDist == 0 {
						dirDist = 1
					}

					p.VX = (dirX / dirDist) * r.config.ReflectedMissileSpeed
					p.VY = (dirY / dirDist) * r.config.ReflectedMissileSpeed

					// Ownership transfers to the reflector
					p.OwnerID = targetID
					p.TeamID = target.Team
					p.Color = target.Color // change color to reflector
					p.X += (p.VX * dt)
					p.Y += (p.VY * dt)
					hit = false // Don't take damage, projectile stays alive
				}
			} else {
				dist := math.Hypot(p.X-target.X, p.Y-target.Y)
				if dist < p.Radius+target.Radius && target.HP > 0 {
					hit = true
					dmg := float64(r.config.MissileDamage)
					if p.IsSuper {
						dmg = float64(r.config.SuperMissileDamage)
					}
					dmg = dmg * p.DamageMult

					target.HP -= int(math.Round(dmg))
					if target.HP <= 0 {
						target.HP = 0
						// Game over logic will catch this at the top of updateState next frame
					}
				}
			}
		}

		if !hit {
			aliveProjectiles = append(aliveProjectiles, p)
		}
	}
	r.gameState.Projectiles = aliveProjectiles
}

func (r *Room) broadcastState() {
	r.stateMutex.Lock()
	stateJSON, _ := json.Marshal(map[string]interface{}{
		"type":     "state",
		"state":    r.gameState,
		"settings": r.config,
	})
	r.stateMutex.Unlock()

	for client := range r.clients {
		select {
		case client.send <- stateJSON:
		default:
			close(client.send)
			delete(r.clients, client)
		}
	}
}
