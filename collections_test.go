package porttools

import (
	"testing"
	"time"
)

var (
	startingCash = FloatAmount(10000.00)
	port         = NewPortfolio(startingCash)
	newHolding   = new(Position)
	txAmount     Amount
)

func remock() {
	startingCash = FloatAmount(10000.00)
	port = NewPortfolio(startingCash)

	// Setup new holding
	ask := FloatAmount(50.00)
	bid := FloatAmount(49.50)
	bidDatedMetric := &datedMetric{Amount: bid, Date: time.Time{}}
	askDatedMetric := &datedMetric{Amount: ask, Date: time.Time{}}

	newHolding = &Position{
		Ticker:   "GOOGL",
		Volume:   10.00,
		NumTicks: 1,
		BuyPrice: askDatedMetric,
		LastAsk:  askDatedMetric, MaxAsk: askDatedMetric, MinAsk: askDatedMetric,
		LastBid: bidDatedMetric, MaxBid: bidDatedMetric, MinBid: bidDatedMetric,
	}
	txAmount = newHolding.BuyPrice.Amount * newHolding.Volume
}

func TestPortfolio_applyDelta(t *testing.T) {
	remock()

	endCash := DivideAmt(startingCash, FloatAmount(2.00))
	port.applyDelta(-endCash)

	if port.cash != endCash {
		t.Errorf("Expected %d, got %d", endCash, port.cash)
	}
}

func TestPortfolio_AddHolding(t *testing.T) {
	remock()

	err := port.AddHolding(newHolding, -txAmount)
	if err != nil {
		t.Errorf("Expected nil, got %s", err)
	}
	if port.active["GOOGL"].len != 1 {
		t.Errorf("Expected slice of len 1, got %d", port.active["GOOGL"].len)
	}
	if port.active["GOOGL"].totalVolume != newHolding.Volume {
		t.Errorf("Expected total volume of %d, got %d", newHolding.Volume, port.active["GOOGL"].totalVolume)
	}
}
