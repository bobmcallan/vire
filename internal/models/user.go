package models

// User represents a user account stored in vire-server.
type User struct {
	Username         string   `json:"username"`
	Email            string   `json:"email"`
	PasswordHash     string   `json:"password_hash"`
	Role             string   `json:"role"`
	NavexaKey        string   `json:"navexa_key,omitempty"`
	DisplayCurrency  string   `json:"display_currency,omitempty"`
	DefaultPortfolio string   `json:"default_portfolio,omitempty"`
	Portfolios       []string `json:"portfolios,omitempty"`
}
