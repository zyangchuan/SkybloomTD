package gameobject

import "math"

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (p Position) DistanceTo(position Position) float64 {
	return math.Hypot(position.X-p.X, position.Y-p.Y)
}

func (p Position) DirectionTo(position Position) Vector {
	return Vector{
		X: position.X - p.X,
		Y: position.Y - p.Y,
	}.Normalize()
}

type Vector struct {
	X float64
	Y float64
}

func (v Vector) Normalize() Vector {
	length := math.Hypot(v.X, v.Y)
	if length == 0 {
		return Vector{}
	}
	return Vector{
		X: v.X / length,
		Y: v.Y / length,
	}
}

func (v Vector) Rotate(radians float64) Vector {
	sin, cos := math.Sin(radians), math.Cos(radians)
	return Vector{
		X: v.X*cos - v.Y*sin,
		Y: v.X*sin + v.Y*cos,
	}.Normalize()
}
