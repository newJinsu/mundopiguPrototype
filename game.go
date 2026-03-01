package main

// Vector represents a 2D coordinate or direction.
type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// GameSettings holds all configurable parameters for a game session.
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
	MinChargeTime         float64 `json:"minChargeTime"`
	ChargeSpeedMult       float64 `json:"chargeSpeedMult"`
	DashDistance          float64 `json:"dashDistance"`
	DashCooldownLimit     float64 `json:"dashCooldownLimit"`
	ReflectCooldownLimit  float64 `json:"reflectCooldownLimit"`
	AttackCooldownLimit   float64 `json:"attackCooldownLimit"`
	MaxHP                 int     `json:"maxHP"`
	MissileDamage         int     `json:"missileDamage"`
	SuperMissileDamage    int     `json:"superMissileDamage"`
	MaxPlayersPerTeam     int     `json:"maxPlayersPerTeam"`
}

// DefaultConfig provides the default settings for a new game room.
var DefaultConfig = GameSettings{
	BoardWidth:            1920.0,
	BoardHeight:           1080.0,
	PlayerRadius:          50.0,
	BaseSpeed:             500.0,
	MissileSpeed:          1000.0,
	SuperMissileSpeed:     1500.0,
	ReflectedMissileSpeed: 3000.0,
	MissileRadius:         10.0,
	SuperMissileRadius:    20.0,
	MaxChargeTime:         0.5,
	MinChargeTime:         0.15,
	ChargeSpeedMult:       0.5,
	DashDistance:          150.0,
	DashCooldownLimit:     2.0,
	ReflectCooldownLimit:  3.0,
	AttackCooldownLimit:   0.8,
	MaxHP:                 100,
	MissileDamage:         15,
	SuperMissileDamage:    35,
	MaxPlayersPerTeam:     5,
}

// PlayerState represents the current state of a player in a game.
type PlayerState struct {
	ID              string  `json:"id"`
	Team            string  `json:"team"`
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

// Projectile represents a projectile in flight.
type Projectile struct {
	ID         int     `json:"id"`
	OwnerID    string  `json:"ownerId"`
	TeamID     string  `json:"teamId"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	Radius     float64 `json:"radius"`
	Color      string  `json:"color"`
	IsSuper    bool    `json:"isSuper"`
	DamageMult float64 `json:"damageMult"`
}

// GameState is the entire state of a single game room sent to clients.
type GameState struct {
	Status      string                  `json:"status"` // "waiting", "playing", "finished"
	Players     map[string]*PlayerState `json:"players"`
	Projectiles []Projectile            `json:"projectiles"`
	GameOver    bool                    `json:"gameOver"`
	Winner      string                  `json:"winner"`
}

// ClientInput is the structure for processing inputs sent from a client.
type ClientInput struct {
	Type     string        `json:"type,omitempty"`
	RoomID   string        `json:"roomId,omitempty"`
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

// InitMessage is the first payload sent to a connected client.
type InitMessage struct {
	Type     string       `json:"type"`
	Role     string       `json:"role"`
	Width    float64      `json:"width"`
	Height   float64      `json:"height"`
	Settings GameSettings `json:"settings"`
}
