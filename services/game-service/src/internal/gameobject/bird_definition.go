package gameobject

import (
	"errors"
	"math"
)

const (
	BirdTypeSparrow    = "sparrow"
	BirdTypeWoodpecker = "woodpecker"
	BirdTypeEagle      = "eagle"
	BirdTypePeacock    = "peacock"
	BirdTypeFalcon     = "falcon"
	BirdTypeKingfisher = "kingfisher"
	BirdTypePhoenix    = "phoenix"
	BirdTypeSunGod     = "sun_god"

	AttackTypeSingle = "single"
	AttackTypeSplash = "splash"
	AttackTypeRing   = "ring"

	StandardProjectileSpeed = 8.0
	StandardSplashSpread    = DefaultSplashSpreadRadians
)

type BirdDefinition struct {
	Type       string
	Stats      BirdStats
	AttackType string
	Hybrid     bool
}

var birdDefinitionOrder = []string{
	BirdTypeSparrow,
	BirdTypeWoodpecker,
	BirdTypeEagle,
	BirdTypePeacock,
	BirdTypeFalcon,
	BirdTypeKingfisher,
	BirdTypePhoenix,
	BirdTypeSunGod,
}

var birdDefinitions = map[string]BirdDefinition{
	BirdTypeSparrow: {
		Type: BirdTypeSparrow,
		Stats: BirdStats{
			Damage:          20,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        1.0,
			Range:           2.1,
			Cost:            50,
		},
		AttackType: AttackTypeSingle,
	},
	BirdTypeWoodpecker: {
		Type: BirdTypeWoodpecker,
		Stats: BirdStats{
			Damage:          10,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        2.0,
			Range:           2.1,
			Cost:            65,
		},
		AttackType: AttackTypeSingle,
	},
	BirdTypeEagle: {
		Type: BirdTypeEagle,
		Stats: BirdStats{
			Damage:          50,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        0.5,
			Range:           3.2,
			Cost:            130,
		},
		AttackType: AttackTypeSingle,
	},
	BirdTypePeacock: {
		Type: BirdTypePeacock,
		Stats: BirdStats{
			Damage:          20,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        1.0,
			Range:           2.1,
			Spread:          StandardSplashSpread,
			Cost:            90,
		},
		AttackType: AttackTypeSplash,
	},
	BirdTypeFalcon: {
		Type: BirdTypeFalcon,
		Stats: BirdStats{
			Damage:          50,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        0.9,
			Range:           3.6,
			Cost:            50,
		},
		AttackType: AttackTypeSingle,
		Hybrid:     true,
	},
	BirdTypeKingfisher: {
		Type: BirdTypeKingfisher,
		Stats: BirdStats{
			Damage:          20,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        3.0,
			Range:           2.4,
			Spread:          StandardSplashSpread,
			Cost:            50,
		},
		AttackType: AttackTypeSplash,
		Hybrid:     true,
	},
	BirdTypePhoenix: {
		Type: BirdTypePhoenix,
		Stats: BirdStats{
			Damage:          25,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        0.8,
			Range:           2.0,
			Cost:            50,
		},
		AttackType: AttackTypeRing,
		Hybrid:     true,
	},
	BirdTypeSunGod: {
		Type: BirdTypeSunGod,
		Stats: BirdStats{
			Damage:          18,
			ProjectileSpeed: StandardProjectileSpeed,
			FireRate:        3.2,
			Range:           4.5,
			Spread:          math.Pi / 36,
			Pierce:          5,
			Cost:            150,
		},
		AttackType: AttackTypeSplash,
		Hybrid:     true,
	},
}

func BirdDefinitionForType(birdType string) (BirdDefinition, error) {
	definition, ok := birdDefinitions[birdType]
	if !ok {
		return BirdDefinition{}, errors.New("unknown bird type")
	}
	return definition, nil
}

func BirdStatsForType(birdType string) (BirdStats, error) {
	definition, err := BirdDefinitionForType(birdType)
	if err != nil {
		return BirdStats{}, err
	}
	return definition.Stats, nil
}

func AttackBehaviourForType(birdType string) (AttackBehaviour, error) {
	definition, err := BirdDefinitionForType(birdType)
	if err != nil {
		return nil, err
	}
	return AttackBehaviourForAttackType(definition.AttackType)
}

func AttackBehaviourForAttackType(attackType string) (AttackBehaviour, error) {
	switch attackType {
	case AttackTypeSingle:
		return SingleAttack{}, nil
	case AttackTypeSplash:
		return SplashAttack{}, nil
	case AttackTypeRing:
		return RingAttack{}, nil
	default:
		return nil, errors.New("unknown attack type")
	}
}

func AttackTypeForBirdType(birdType string) (string, error) {
	definition, err := BirdDefinitionForType(birdType)
	if err != nil {
		return "", err
	}
	return definition.AttackType, nil
}

func IsHybridBirdType(birdType string) bool {
	definition, ok := birdDefinitions[birdType]
	return ok && definition.Hybrid
}

func BirdTypes() []string {
	return append([]string{}, birdDefinitionOrder...)
}
