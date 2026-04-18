package game

import (
	"math"
	"time"
)

func (s *Server) maxHealthForMass(mass float64) float64 {
	value := normalizedLogCurve(
		s.cfg.HealthBase,
		s.cfg.HealthScale,
		s.cfg.HealthMassScale,
		s.cfg.StartingMass,
		mass,
	)
	return math.Max(value, 1)
}

func (s *Server) radiusForMass(mass float64) float64 {
	value := normalizedLogCurve(
		s.cfg.RadiusBase,
		s.cfg.RadiusScale,
		s.cfg.RadiusMassScale,
		s.cfg.StartingMass,
		mass,
	)
	return math.Max(value, 1)
}

func normalizedLogCurve(base, scale, massScale, startingMass, mass float64) float64 {
	if massScale <= 0 {
		massScale = 1
	}
	safeMass := math.Max(mass, 0.01)
	safeStartingMass := math.Max(startingMass, 0.01)
	return base + scale*(math.Log1p(safeMass/massScale)-math.Log1p(safeStartingMass/massScale))
}

func (s *Server) rescaleHealthForMassChange(oldHealth, oldMass, newMass float64) float64 {
	oldMaxHealth := s.maxHealthForMass(oldMass)
	newMaxHealth := s.maxHealthForMass(newMass)
	if oldMaxHealth <= 0 {
		return clamp(oldHealth, 0, newMaxHealth)
	}
	return clamp((oldHealth/oldMaxHealth)*newMaxHealth, 0, newMaxHealth)
}

func (s *Server) setPlayerMassPreservingHealth(player *Player, newMass float64) {
	oldMass := player.Mass
	oldHealth := player.Health
	player.Mass = math.Max(newMass, 0.01)
	if !player.Alive && player.Health <= 0 {
		return
	}
	player.Health = s.rescaleHealthForMassChange(oldHealth, oldMass, player.Mass)
}

func (s *Server) applyCollectiblePickup(player *Player, collectibleMass float64) (float64, float64) {
	massGain := math.Max(collectibleMass, 0)
	oldMass := player.Mass
	oldMaxHealth := s.maxHealthForMass(oldMass)
	newMass := oldMass + massGain
	newMaxHealth := s.maxHealthForMass(newMass)
	player.Mass = math.Max(newMass, 0.01)
	if !player.Alive && player.Health <= 0 {
		return massGain, 0
	}

	oldHealth := player.Health
	player.Health = clamp(oldHealth+math.Max(newMaxHealth-oldMaxHealth, 0), 0, newMaxHealth)
	return massGain, math.Max(player.Health-oldHealth, 0)
}

func (s *Server) crashDamageForPlayer(player *Player) float64 {
	return s.maxHealthForMass(player.Mass) * s.cfg.CrashDamagePct
}

func (s *Server) killMassTransfer(victimMass float64) float64 {
	return math.Max(victimMass, 0) * s.cfg.KillMassTransferPct
}

func (s *Server) killHealAmount(killerMass float64) float64 {
	return s.maxHealthForMass(killerMass) * s.cfg.KillHealPct
}

func (s *Server) passiveHealDelta(dtSeconds float64) float64 {
	return s.cfg.PassiveHealPerSecond * dtSeconds
}

func (s *Server) respawnMass(preDeathMass float64) float64 {
	return math.Max(s.cfg.StartingMass, preDeathMass*s.cfg.RespawnRetentionPct)
}

func (s *Server) isInvulnerable(player *Player, now time.Time) bool {
	return !player.SpawnInvulnerableUntil.IsZero() && now.Before(player.SpawnInvulnerableUntil)
}

func stablePairKey(leftID, rightID string) string {
	if leftID < rightID {
		return leftID + "|" + rightID
	}
	return rightID + "|" + leftID
}
