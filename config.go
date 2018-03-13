package porttools

import (
	"encoding/json" // will be used later
	"os"
	"time"
)

//NOTE: In a contemporary electronic market (circa 2009), low latency trade processing time was qualified as under 10 milliseconds, and ultra-low latency as under 1 millisecond

// LoadConfig uses a Json File to populate details regarding configuration.
func LoadConfig(filename string) (config *Config, err error) {
	file, fileErr := os.Open(filename)
	defer file.Close()
	if fileErr != nil {
		return nil, fileErr
	}

	decoder := json.NewDecoder(file)
	if decodeErr := decoder.Decode(&config); decodeErr != nil {
		return nil, decodeErr
	}

	return config, nil
}

// Config is used as a struct store store configuration data in.
type Config struct {
	File struct {
		Glob        string `json:"fileGlob"`
		Delim       rune   `json:"delim"`
		ExampleDate string `json:"exampleDate"`
		Columns     struct {
			Ticker    int       `json:"ticker"`
			Timestamp time.Time `json:"timestamp"`
			Volume    int       `json:"volume"`
			BidPrice  int       `json:"bidPrice"`
			BidSize   int       `json:"bidSize"`
			AskPrice  int       `json:"askPrice"`
			AskSize   int       `json:"askSize"`
		} `json:"columns"`
	} `json:"file"`

	Backtest struct {
		IgnoreSecurities []string `json:"ignoreSecurities"`
		Slippage         float64  `json:"slippage"`
		Commission       float64  `json:"commission"`
	} `json:"backtest"`

	Simulation struct {
		StartDate time.Time   `json:"startDate"`
		EndDate   time.Time   `json:"endDate"`
		BarRate   BarDuration `json:"barRate"`
		//  IngestRate measures how many bars to skip
		// IngestRate BarDuration `json:"ingestRate"`
	} `json:"simulation"`

	Benchmark struct {
		Use    bool `json:"use"`
		Update bool `json:"update"`
	} `json:"benchmark"`
}

// BarDuration is used to register tick intake.
type BarDuration time.Duration

// // QUESTION: is this function needed?
// func (cfg simConfig) dataFiles(pattern string) ([]string, error) {
// 	return filepath.Glob(pattern)
// 	// QUESTION: is this if statement necessary if Glob is creating
// 	// 		error for us?
// 	// if files, err := filepath.Glob(pattern); err != nil {
// 	// 	return files, err
// 	// } else {
// 	// 	return files, nil
// 	// }
// }
