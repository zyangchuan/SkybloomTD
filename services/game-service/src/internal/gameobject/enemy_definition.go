package gameobject

import "errors"

const (
	EnemyTypeSmog  = "smog"
	EnemyTypeJunk  = "junk"
	EnemyTypeNoise = "noise"
)

type EnemyDefinition struct {
	Type  string
	Stats EnemyStats
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
			Health: 40,
			Speed:  0.8,
		},
	},
	EnemyTypeJunk: {
		Type: EnemyTypeJunk,
		Stats: EnemyStats{
			Health: 300,
			Speed:  0.3,
		},
	},
	EnemyTypeNoise: {
		Type: EnemyTypeNoise,
		Stats: EnemyStats{
			Health: 15,
			Speed:  1.4,
		},
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

func EnemyTypes() []string {
	return append([]string{}, enemyDefinitionOrder...)
}
