package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Constants

// Data Structures
type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type GameSettings struct {
	BoardWidth            float64 `json:"boardWidth"`
	BoardHeight           float64 `json:"boardHeight"`
	PlayerRadius          float64 `json:"playerRadius"`
	BaseSpeed             float64 `json:"baseSpeed"`
	MissileSpeed          float64 `json:"missileSpeed"`
	SuperMissileSpeed     float64 `json:"superMissileSpeed"`
	ReflectedMissileSpeed float64 `json:"reflectedMissileSpeed"`
	MissileRadius         float64 `json:"missileRadius"`
	SuperMissileRadius    float64 `json:"superMissileRadius"`
	MaxChargeTime         float64 `json:"maxChargeTime"`
	ChargeSpeedMult       float64 `json:"chargeSpeedMult"`
	DashDistance          float64 `json:"dashDistance"`
	DashCooldownLimit     float64 `json:"dashCooldownLimit"`
	ReflectCooldownLimit  float64 `json:"reflectCooldownLimit"`
	AttackCooldownLimit   float64 `json:"attackCooldownLimit"`
	MaxHP                 int     `json:"maxHP"`
	MissileDamage         int     `json:"missileDamage"`
	SuperMissileDamage    int     `json:"superMissileDamage"`
}

var globalConfig = GameSettings{
	BoardWidth:            1600.0,
	BoardHeight:           900.0,
	PlayerRadius:          70.0,
	BaseSpeed:             400.0,
	MissileSpeed:          2000.0,
	SuperMissileSpeed:     2300.0,
	ReflectedMissileSpeed: 3000.0,
	MissileRadius:         12.0,
	SuperMissileRadius:    18.0,
	MaxChargeTime:         0.4,
	ChargeSpeedMult:       0.5,
	DashDistance:          200.0,
	DashCooldownLimit:     2.0,
	ReflectCooldownLimit:  4.0,
	AttackCooldownLimit:   0.5,
	MaxHP:                 100,
	MissileDamage:         20,
	SuperMissileDamage:    40,
}

type PlayerState struct {
	ID              string  `json:"id"`
	X               float64 `json:"x"`
	Y               float64 `json:"y"`
	Radius          float64 `json:"radius"`
	Color           string  `json:"color"`
	HP              int     `json:"hp"`
	MaxHP           int     `json:"maxHP"`
	IsCharging      bool    `json:"isCharging"`
	ChargeTime      float64 `json:"chargeTime"`
	MaxCharge       float64 `json:"maxChargeTime"`
	IsDashing       bool    `json:"isDashing"`
	DashTimer       float64 `json:"dashTimer"`
	DashCooldown    float64 `json:"dashCooldown"`
	IsReflecting    bool    `json:"isReflecting"`
	ReflectTimer    float64 `json:"reflectTimer"`
	ReflectCooldown float64 `json:"reflectCooldown"`
	AttackCooldown  float64 `json:"attackCooldown"`
	DashDir         Vector  `json:"dashDir"`
	HasDamageBuff   bool    `json:"hasDamageBuff"`
}

type Projectile struct {
	ID         int     `json:"id"`
	OwnerID    string  `json:"ownerId"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Radius     float64 `json:"radius"`
	Color      string  `json:"color"`
	IsSuper    bool    `json:"isSuper"`
	DamageMult float64 `json:"damageMult"`
}

type GameState struct {
	Players     map[string]*PlayerState `json:"players"`
	Projectiles []Projectile            `json:"projectiles"`
	GameOver    bool                    `json:"gameOver"`
	Winner      string                  `json:"winner"`
}

var (
	gameState = GameState{
		Players:     make(map[string]*PlayerState),
		Projectiles: make([]Projectile, 0),
		GameOver:    false,
	}
	stateMutex   sync.Mutex
	clients      = make(map[*websocket.Conn]string) // mapping conn to player ID ("p1", "p2")
	clientsMutex sync.Mutex
	projCounter  = 0
)

// Input from client
type ClientInput struct {
	Type     string        `json:"type,omitempty"`
	Up       bool          `json:"up"`
	Down     bool          `json:"down"`
	Left     bool          `json:"left"`
	Right    bool          `json:"right"`
	Dash     bool          `json:"dash"`
	IsAttack bool          `json:"isAttack"` // mouse button down
	Reflect  bool          `json:"reflect"`  // reflect skill
	MouseX   float64       `json:"mouseX"`
	MouseY   float64       `json:"mouseY"`
	Settings *GameSettings `json:"settings,omitempty"`
}

type InitMessage struct {
	Type     string       `json:"type"`
	Role     string       `json:"role"`
	Width    float64      `json:"width"`
	Height   float64      `json:"height"`
	Settings GameSettings `json:"settings"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func initGame() {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	gameState.Players["p1"] = &PlayerState{
		ID:        "p1",
		X:         globalConfig.BoardWidth * 0.25,
		Y:         globalConfig.BoardHeight / 2,
		Radius:    globalConfig.PlayerRadius,
		Color:     "#3498db",
		HP:        globalConfig.MaxHP,
		MaxHP:     globalConfig.MaxHP,
		MaxCharge: globalConfig.MaxChargeTime,
	}
	gameState.Players["p2"] = &PlayerState{
		ID:        "p2",
		X:         globalConfig.BoardWidth * 0.75,
		Y:         globalConfig.BoardHeight / 2,
		Radius:    globalConfig.PlayerRadius,
		Color:     "#e74c3c",
		HP:        globalConfig.MaxHP,
		MaxHP:     globalConfig.MaxHP,
		MaxCharge: globalConfig.MaxChargeTime,
	}
	gameState.Projectiles = make([]Projectile, 0)
	gameState.GameOver = false
	gameState.Winner = ""
}

func main() {
	initGame()

	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", handleConnections)

	go gameLoop()

	fmt.Println("Server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	clientsMutex.Lock()
	var role string
	// Assign role based on current connected
	if len(clients) == 0 {
		role = "p1"
	} else if len(clients) == 1 {
		// check who is already connected
		for _, v := range clients {
			if v == "p1" {
				role = "p2"
			} else {
				role = "p1"
			}
		}
	} else {
		// Full, make them spectator (or reject)
		role = "spectator"
	}

	if role == "p1" || role == "p2" || role == "spectator" {
		clients[ws] = role
	}
	clientsMutex.Unlock()

	defer func() {
		clientsMutex.Lock()
		delete(clients, ws)
		clientsMutex.Unlock()
		ws.Close()
	}()

	// Send Init
	initMsg := InitMessage{
		Type:     "init",
		Role:     role,
		Width:    globalConfig.BoardWidth,
		Height:   globalConfig.BoardHeight,
		Settings: globalConfig,
	}
	ws.WriteJSON(initMsg)

	// Listen for inputs
	for {
		var input ClientInput
		err := ws.ReadJSON(&input)
		if err != nil {
			break
		}

		if input.Type == "restart" {
			initGame()
			continue
		}

		if input.Type == "update_settings" && input.Settings != nil {
			stateMutex.Lock()
			globalConfig = *input.Settings
			for _, p := range gameState.Players {
				p.Radius = globalConfig.PlayerRadius
				p.MaxCharge = globalConfig.MaxChargeTime
				p.MaxHP = globalConfig.MaxHP
				if p.HP > p.MaxHP {
					p.HP = p.MaxHP
				}
			}
			stateMutex.Unlock()
			continue
		}

		if role == "p1" || role == "p2" {
			processInput(role, input)
		}
	}
}

func processInput(role string, input ClientInput) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	p := gameState.Players[role]
	if p == nil || gameState.GameOver {
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
		p.ReflectCooldown = globalConfig.ReflectCooldownLimit
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
		p.DashCooldown = globalConfig.DashCooldownLimit

		if dx == 0 && dy == 0 {
			// Dash towards mouse if not moving
			mx := input.MouseX - p.X
			my := input.MouseY - p.Y
			mlen := math.Hypot(mx, my)
			if mlen > 0 {
				p.DashDir = Vector{X: mx / mlen, Y: my / mlen}
			} else {
				if role == "p1" {
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
		if input.IsAttack && p.AttackCooldown <= 0 {
			p.IsCharging = true
			p.ChargeTime += dt
			if p.ChargeTime > globalConfig.MaxChargeTime {
				p.ChargeTime = globalConfig.MaxChargeTime
			}
		} else if p.IsCharging {
			// Release -> Shoot
			shoot(role, p, input.MouseX, input.MouseY)
			p.IsCharging = false
			p.ChargeTime = 0
			p.AttackCooldown = globalConfig.AttackCooldownLimit
		}
	}

	// Apply movement
	currentSpeed := globalConfig.BaseSpeed
	if p.IsDashing {
		currentSpeed = globalConfig.DashDistance / 0.2 // Since DashTimer is 0.2
		dx = p.DashDir.X
		dy = p.DashDir.Y
	} else if p.IsCharging {
		currentSpeed = globalConfig.BaseSpeed * globalConfig.ChargeSpeedMult
	} else if p.IsReflecting {
		currentSpeed = globalConfig.BaseSpeed * 0.3 // Slowed slightly while reflecting
	}

	p.X += dx * currentSpeed * dt
	p.Y += dy * currentSpeed * dt

	// Bounds checking
	p.Y = math.Max(p.Radius, math.Min(globalConfig.BoardHeight-p.Radius, p.Y))
	if role == "p1" {
		p.X = math.Max(p.Radius, math.Min(globalConfig.BoardWidth/2-p.Radius, p.X))
	} else {
		p.X = math.Max(globalConfig.BoardWidth/2+p.Radius, math.Min(globalConfig.BoardWidth-p.Radius, p.X))
	}
}

func shoot(role string, p *PlayerState, mx float64, my float64) {
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

	isSuper := p.ChargeTime >= globalConfig.MaxChargeTime
	pSpeed := globalConfig.MissileSpeed
	pRadius := globalConfig.MissileRadius
	pColor := p.Color

	if isSuper {
		pSpeed = globalConfig.SuperMissileSpeed
		pRadius = globalConfig.SuperMissileRadius
		pColor = "#f1c40f"
	}

	projCounter++
	gameState.Projectiles = append(gameState.Projectiles, Projectile{
		ID:         projCounter,
		OwnerID:    role,
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

func gameLoop() {
	ticker := time.NewTicker(time.Millisecond * 16) // ~60 FPS
	defer ticker.Stop()

	for range ticker.C {
		updateState()
		broadcastState()
	}
}

func updateState() {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	if gameState.GameOver {
		return
	}

	dt := 1.0 / 60.0

	// Projectiles logic
	aliveProjectiles := make([]Projectile, 0)

	for _, p := range gameState.Projectiles {
		p.X += p.VX * dt
		p.Y += p.VY * dt

		// Map OOB
		if p.X < 0 || p.X > globalConfig.BoardWidth || p.Y < 0 || p.Y > globalConfig.BoardHeight {
			continue
		}

		hit := false
		for targetID, target := range gameState.Players {
			if targetID == p.OwnerID {
				continue // don't hit self (unless it reflects, but standard is no)
			}

			if target.IsReflecting {
				dist := math.Hypot(p.X-target.X, p.Y-target.Y)
				if dist < p.Radius+target.Radius+10 { // Reflection shield radius padding
					// 저스트 디펜스 버프 획득
					target.HasDamageBuff = true

					// Reflect the projectile towards the original owner
					originalOwnerID := p.OwnerID
					originalOwner := gameState.Players[originalOwnerID]

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

					p.VX = (dirX / dirDist) * globalConfig.ReflectedMissileSpeed
					p.VY = (dirY / dirDist) * globalConfig.ReflectedMissileSpeed

					// Ownership transfers to the reflector
					p.OwnerID = targetID
					p.Color = target.Color // change color to reflector
					// Give it a tiny push outwards so it doesn't immediately hit the shield again in weird angle
					p.X += (p.VX * dt)
					p.Y += (p.VY * dt)
					hit = false // Don't take damage, projectile stays alive
				}
			} else {
				dist := math.Hypot(p.X-target.X, p.Y-target.Y)
				if dist < p.Radius+target.Radius {
					hit = true
					dmg := float64(globalConfig.MissileDamage)
					if p.IsSuper {
						dmg = float64(globalConfig.SuperMissileDamage)
					}
					dmg = dmg * p.DamageMult

					target.HP -= int(math.Round(dmg))
					if target.HP <= 0 {
						target.HP = 0
						gameState.GameOver = true
						if p.OwnerID == "p1" {
							gameState.Winner = "🔵 Player 1 승리!"
						} else {
							gameState.Winner = "🔴 Player 2 승리!"
						}
					}
				}
			}
		}

		if !hit {
			aliveProjectiles = append(aliveProjectiles, p)
		}
	}
	gameState.Projectiles = aliveProjectiles
}

func broadcastState() {
	stateMutex.Lock()
	stateJSON, _ := json.Marshal(map[string]interface{}{
		"type":     "state",
		"state":    gameState,
		"settings": globalConfig,
	})
	stateMutex.Unlock()

	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for ws := range clients {
		err := ws.WriteMessage(websocket.TextMessage, stateJSON)
		if err != nil {
			ws.Close()
			delete(clients, ws)
		}
	}
}
