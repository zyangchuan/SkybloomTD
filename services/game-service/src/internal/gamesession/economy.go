package gamesession

type Economy struct {
	Essence int `json:"essence"`
}

func NewEconomy(essence int) Economy {
	if essence < 0 {
		essence = 0
	}
	return Economy{Essence: essence}
}

func (e *Economy) Consume(amount int) bool {
	if e == nil || amount < 0 || e.Essence < amount {
		return false
	}
	e.Essence -= amount
	return true
}

func (e *Economy) Add(amount int) {
	if e == nil || amount <= 0 {
		return
	}
	e.Essence += amount
}
