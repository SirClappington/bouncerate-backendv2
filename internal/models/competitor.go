package models

// Product represents a product offered by a competitor.
type Product struct {
	Name     string `json:"name"`
	Price    string `json:"price"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

// Competitor represents a competitor in the market.
type Competitor struct {
	Name     string    `json:"name"`
	Website  string    `json:"website"`
	Products []Product `json:"products"`
}
