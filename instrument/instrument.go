package instrument

import (
	"time"

	"github.com/jakeschurch/porttools/utils"
)

// Financial is an interface that is used for types that are embedded,
// as well as update its own metrics.
type Financial interface {
	Update(Tick) error
	GetUnderlying() Financial
	Volume(utils.Amount) utils.Amount
	Ticker() string
}

// Tick structs holds information about a financial asset at a specific point in time.
type Tick struct {
	Ticker           string
	Bid, Ask         utils.Amount
	BidSize, AskSize utils.Amount
	Timestamp        time.Time
}

// ------------------------------------------------------------------

// Instrument is the base type of a financial widget.
type Instrument struct {
	ticker string
	volume utils.Amount
	Nticks uint
}

// NewInstrument instantities a new struct of type Instrument.
func NewInstrument(ticker string, volume utils.Amount) *Instrument {
	instrument := new(Instrument)
	instrument.ticker = ticker
	instrument.Nticks = 0
	instrument.Volume(volume)
	return instrument
}

// GetUnderlying returns nil.
func (i Instrument) GetUnderlying() Financial {
	return nil
}

// Volume can be used as a get/set method if 0 is delta.
func (i *Instrument) Volume(delta utils.Amount) utils.Amount {
	i.volume = i.volume + delta
	return i.volume
}

func (i Instrument) Ticker() string {
	return i.ticker
}

// Update for an instrument is used to  implement Financial interface
func (i *Instrument) Update(t Tick) error {
	return nil
}

// ------------------------------------------------------------------

// Asset is tradeable instrument type.
type Asset struct {
	*Instrument

	// AvgBid, AvgAsk   utils.Amount -> can be calculated
	LastBid, LastAsk *utils.DatedMetric
	MaxBid, MaxAsk   *utils.DatedMetric
	MinBid, MinAsk   *utils.DatedMetric
}

// GetUnderlying returns an asset's embedded Instrument type.
func (a Asset) GetUnderlying() Financial {
	return a.Instrument
}

// NewAsset instantiaties a new struct of type Asset.
func NewAsset(ticker string, bid, ask, volume utils.Amount, timestamp time.Time) *Asset {
	assetBid := &utils.DatedMetric{Amount: bid, Date: timestamp}
	assetAsk := &utils.DatedMetric{Amount: ask, Date: timestamp}

	return &Asset{
		Instrument: &Instrument{ticker: ticker, volume: volume, Nticks: 1},

		// AvgBid: assetBid.Amount, AvgAsk: assetAsk.Amount,
		LastBid: assetBid, MaxBid: assetBid, MinBid: assetBid,
		LastAsk: assetAsk, MaxAsk: assetAsk, MinAsk: assetAsk,
	}
}

func (a Asset) Update(t Tick) error {
	return a.update(t)
}

// Update uses new tick data to update an asset's metrics.
func (a *Asset) update(t Tick) error {
	// update bid metrics
	// a.AvgBid = utils.Avg(a.AvgBid, a.Nticks, t.Bid)
	a.LastBid = &utils.DatedMetric{Amount: t.Bid, Date: t.Timestamp}
	a.MaxBid = utils.Max(a.MaxBid, t.Bid, t.Timestamp)
	a.MinBid = utils.Min(a.MinBid, t.Bid, t.Timestamp)

	// update ask metrics
	// a.AvgAsk = utils.Avg(a.AvgAsk, a.Nticks, t.Ask)
	a.LastAsk = &utils.DatedMetric{Amount: t.Ask, Date: t.Timestamp}
	a.MaxAsk = utils.Max(a.MaxAsk, t.Ask, t.Timestamp)
	a.MinAsk = utils.Min(a.MinAsk, t.Ask, t.Timestamp)

	a.Nticks++
	return nil
}

// ------------------------------------------------------------------

// Holding structs refer the holding of a financial asset.
type Holding struct {
	*Instrument
	BuyPrice, SellPrice *utils.DatedMetric
}

// NewHolding instantities struct of type Holding.
func NewHolding(instrument *Instrument, buyPrice *utils.DatedMetric) *Holding {
	return &Holding{
		Instrument: instrument,
		BuyPrice:   buyPrice,
	}
}

// GetUnderlying method of Holding returns an instrument.Instrument type.
func (h Holding) GetUnderlying() Financial {
	return h.Instrument
}

// ------------------------------------------------------------------

// Security structs hold information regarding a financial asset for the entire
// life of the financial asset in a trading environment. Because a Security struct
// holds aggregate information regarding a financial asset, it is embedded into an Index or Benchmark.
type Security struct {
	*Asset
	BuyPrice, SellPrice *utils.DatedMetric
}

// NewSecurity instantiates a security object from Tick data.
func NewSecurity(buy, sell *utils.DatedMetric, asset Asset) *Security {
	return &Security{
		Asset:    &asset,
		BuyPrice: buy, SellPrice: sell,
	}
}

// GetUnderlying method for security returns an instrument.Asset type.
func (s Security) GetUnderlying() Financial {
	return s.Asset
}

// ------------------------------------------------------------------
