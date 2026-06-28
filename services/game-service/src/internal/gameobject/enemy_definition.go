package gameobject

import "errors"

const (
	EnemyTypeSmog  = "smog"
	EnemyTypeJunk  = "junk"
	EnemyTypeNoise = "noise"
)

type EnemyDefinition struct {
	Type          string
	Stats         EnemyStats
	EssenceReward int
}

var enemyDefinitionOrder = []string{
	EnemyTypeSmog,
	EnemyTypeJunk,
	EnemyTypeNoise,
}

var enemyDefinitions = map[string]EnemyDefinition{
	EnemyTypeSmog: {
		Type: EnemyTypeSmog,
		Stats: EnemyStats{
			Health: 20,
			Speed:  0.8,
		},
		EssenceReward: 5,
	},
	EnemyTypeJunk: {
		Type: EnemyTypeJunk,
		Stats: EnemyStats{
			Health: 500,
			Speed:  0.3,
		},
		EssenceReward: 30,
	},
	EnemyTypeNoise: {
		Type: EnemyTypeNoise,
		Stats: EnemyStats{
			Health: 10,
			Speed:  1.4,
		},
		EssenceReward: 2,
	},
}

func EnemyDefinitionForType(enemyType string) (EnemyDefinition, error) {
	definition, ok := enemyDefinitions[enemyType]
	if !ok {
		return EnemyDefinition{}, errors.New("unknown enemy type")
	}
	return definition, nil
}

func EnemyStatsForType(enemyType string) (EnemyStats, error) {
	definition, err := EnemyDefinitionForType(enemyType)
	if err != nil {
		return EnemyStats{}, err
	}
	return definition.Stats, nil
}

func EnemyEssenceRewardForType(enemyType string) (int, error) {
	definition, err := EnemyDefinitionForType(enemyType)
	if err != nil {
		return 0, err
	}
	return definition.EssenceReward, nil
}

func EnemyTypes() []string {
	return append([]string{}, enemyDefinitionOrder...)
}
