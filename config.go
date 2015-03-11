package inlet_http

type Config struct {
	Address    string `json:"address"`
	Domain     string `json:"domain"`
	Timeout    int64  `json:"timeout"`
	EnableStat bool   `json:"enable_stat"`
}
