//Package config store config information in trade service
package config

//Config structure represents config structure in trade service
type Config struct {
	PostgresURL       string `env:"POSTGRESURL" envDefault:"postgresql://postgres:passwd@localhost:5432/test"`
	TradeServerPort   string `env:"TRADESERVER" envDefault:"localhost:8091"`
	BalanceServerPort string `env:"BALANCESERVER" envDefault:"localhost:8085"`
	PriceServerPort   string `env:"PRICESERVER" envDefault:"localhost:8083"`
}
