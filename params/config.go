package params

type ChainConfig struct {
	ChainID           int
	NodeID            string
	ShardID           string
	Shard_num         int
	Malicious_num     int    // per shard
	Path              string // input file path
	Block_interval    int    // millisecond
	MaxBlockSize      int
	Relay_interval    int
	MaxRelayBlockSize int
	MinRelayBlockSize int
	Inject_speed      int // tx count per second
}

var (
	ClientAddr = "127.0.0.1:8200"
	NodeTable  = map[string]map[string]string{}

	ShardTable = map[string]int{
		"S0": 0,
		"S1": 1,
	}
	ShardTableInt2Str = map[int]string{
		0: "S0",
		1: "S1",
	}

	Config = &ChainConfig{
		ChainID:           77,
		Block_interval:    5000,
		MaxBlockSize:      10,
		MaxRelayBlockSize: 10,
		MinRelayBlockSize: 1,
		Inject_speed:      4,
		Relay_interval:    5000,
	}

	Init_addrs = []string{
		"171382ed4571b1084bb5963053203c237dba6da9",
		"2185bf3bfda43894efdcc1a3f4a99a7f160bc123",
		"374be1a1d1ac0ff350dc9d0a0be3d059c7082791",
		"42fd9ff72a798780c0dffc68f89b64ba240240dd",
	}
	Init_balance string = "100000000000000000000000000000000000000000000" //40个0
)
