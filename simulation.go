package porttools

import (
	"bufio"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jakeschurch/porttools/collection/portfolio"
	"github.com/jakeschurch/porttools/config"
	"github.com/jakeschurch/porttools/instrument"
	"github.com/jakeschurch/porttools/order"
	"github.com/jakeschurch/porttools/output"
	"github.com/jakeschurch/porttools/utils"
)

var (
	oms       *OMS
	port      *portfolio.Portfolio
	prfmLog   *PrfmLog
	benchmark *benchmark.Index
	strategy  Strategy
)

func init() {
	oms = NewOMS()
	port = portfolio.New()
	positionLog = output.NewPositionLog()
	benchmark = benchmark.NewIndex()
}

type QueryFunc func() *interface{}

// Get is a function that allows end-users to access private structs when implementing an algorithm.
func (sim *Simulation) Query(fn ...QueryFunc) []*interface{} {
	retrieved := make([]interface{}, 0)

	for i := range fn {
		retrieved = append(retrieved, fn[i])
	}
	return retrieved
}

// Port returns the portfolio struct being used in the simulation.
func (sim *Simulation) Port() *portfolio.Portfolio {
	return port
}

// OMS returns the Order Management System being used in the simulation.
func (sim *Simulation) OMS() *OMS {
	return oms
}

// sim.PrfmLog()

// IDEA: init statement to clear logic?

var (
	// ErrInvalidFileGlob indiciates that no files could be found from given glob
	ErrInvalidFileGlob = errors.New("No files could be found from file glob")

	// ErrInvalidFileDelim is thrown when file delimiter is not able to be parsed
	ErrInvalidFileDelim = errors.New("File delimiter could not be parsed")
)

// LoadAlgorithm ensures that an Algorithm interface is implemented in the Simulation pipeline to be used by other functions.
func (sim *Simulation) LoadAlgorithm(algo algorithm.Algorithm) bool {
	sim.strategy = strategy.New(algo, make([]string, 0))
	return true
}

// NewSimulation is a constructor for the Simulation data type,
// and a pre-processor function for the embedded types.
func NewSimulation(algo algorithm.Algorithm, cfgFile string) (*Simulation, error) {
	cfg, cfgErr := config.Load(cfgFile)
	if cfgErr != nil {
		log.Fatal("Config error reached: ", cfgErr)
		return nil, cfgErr
	}

	startingCash := utils.FloatAmount(cfg.Backtest.StartCashAmt)
	sim := &Simulation{
		config: *cfg,

		strategy: strategy.New(algo, make([]string, 0)),
		// Channels
		processChan: make(chan *instrument.Tick),
		tickChan:    make(chan *instrument.Tick),
		errChan:     make(chan error),
	}
	return sim, nil
}

// Simulation embeds all data structs necessary for running a backtest of an algorithmic strategy.
type Simulation struct {
	mu          sync.RWMutex
	config      config.Config
	processChan chan *instrument.Tick
	tickChan    chan *instrument.Tick
	errChan     chan error
}

// Run acts as the simulation's primary pipeline function; directing everything to where it needs to go.
func (sim *Simulation) Run() error {
	log.Println("Starting sim...")
	if sim.strategy.Algorithm == nil {
		log.Fatal("Algorithm needs to be implemented by end-user")
	}

	done := make(chan struct{})
	go func() {
		cachedTicks := make([]*instrument.Tick, 0)

		for sim.tickChan != nil {
			tick, ok := <-sim.tickChan
			if !ok {
				if tick != nil {
					cachedTicks = append(cachedTicks, tick)
					// assuming rest of ticks are nil, loop over and process the remaining ticks
				} else {
					for i := range cachedTicks {
						sim.process(cachedTicks[i])
					}
					break
				}
			}
			if tick != nil {
				sim.process(tick)
			}
		}
		close(done)
	}()

	log.Println("loading input...")
	fileName, fileDate := fileInfo(sim.config)

	// DO NOT REVIEW
	colConfig := colConfig{tick: sim.config.File.Columns.Ticker,
		bid:      sim.config.File.Columns.Bid,
		bidSz:    sim.config.File.Columns.BidSize,
		ask:      sim.config.File.Columns.Ask,
		askSz:    sim.config.File.Columns.AskSize,
		filedate: fileDate,
		timeUnit: sim.config.File.TimestampUnit,
	}

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	worker := newWorker(colConfig)
	go worker.run(sim.tickChan, file)

	<-done
	log.Println(len(sim.prfmLog.ClosedHoldings))
	log.Println(len(sim.prfmLog.ClosedOrders))
	output.GetResults(sim.prfmLog.ClosedHoldings, sim.benchmark, sim.config.Simulation.OutFmt)

	return nil
}

func fileInfo(cfg config.Config) (string, time.Time) {
	fileGlob, err := filepath.Glob(cfg.File.Glob)
	if err != nil || len(fileGlob) == 0 {
		// return ErrInvalidFileGlob
	}
	file := fileGlob[0]
	lastUnderscore := strings.LastIndex(file, "_")
	fileDate := file[lastUnderscore+1:]

	lastDate, dateErr := time.Parse(cfg.File.ExampleDate, fileDate)
	if dateErr != nil {
		log.Fatal("Date cannot be parsed")
	}
	filedate := lastDate
	return file, filedate
}

func (sim *Simulation) updateBenchmark(tick instrument.Tick) error {
	// Update benchmark metrics
	if err := sim.benchmark.UpdateMetrics(tick); err != nil {
		if err == benchmark.ErrNoSecurityExists {
			sim.benchmark.AddNew(tick)
		} else {
			return err
		}
	}
	return nil
}
func (sim *Simulation) buyOrderCheck(tick instrument.Tick) (*order.Order, utils.Amount, error) {
	// Add entry order if it meets valid order logic
	newOrder, err := sim.strategy.CheckEntryLogic(sim.port, tick)
	if err != nil {
		return nil, 0, err
	}
	txAmount, err := sim.oms.addOrder(newOrder)
	if err != nil {
		return nil, 0, err
	}
	return newOrder, txAmount, nil
}
func (sim *Simulation) addToPortfolio(order *order.Order, txAmount utils.Amount) error {
	// create new position from order
	newPos := order.ToPosition()
	// add new position (holding) and change in cash from order to portfolio
	if err := sim.port.AddHolding(newPos, txAmount); err != nil {
		return err
	}
	return nil
}

func (sim *Simulation) sellOrderCheck(tick *instrument.Tick) error {
	// Check if open order with same ticker exists
	matchedOrders, err := sim.oms.existsInOrders(tick.Ticker)
	if len(matchedOrders) > 0 {
		// TODO check err
		sim.port.UpdateMetrics(*tick)
	}
	if len(matchedOrders) == 0 {
		return err
	}
	for i := range matchedOrders {

		newClosedOrder, err := sim.strategy.CheckExitLogic(sim.port, matchedOrders[i], *tick)
		if err == nil {
			sim.createSale(newClosedOrder, tick)
		}
	}
	return nil
}

func (sim *Simulation) createSale(newClosedOrder *order.Order, tick *instrument.Tick) error {
	txAmount, ClosedHoldings, _ := sim.oms.TransactSell(
		newClosedOrder,
		sim.config.Simulation.Costmethod,
		sim.port)

	// Update held Cash amount in portfolio
	sim.port.ApplyDelta(txAmount)

	// // Delete holding slice from portfolio active holdings map if now empty
	// if deleteSlice {
	// 	sim.port.Lock()
	// 	delete(sim.port.active, tick.Ticker)
	// 	sim.port.Unlock()
	// }

	// Add closed positions (holdings) to performance log
	for _, closedPos := range ClosedHoldings {
		sim.prfmLog.AddHolding(closedPos)
	}

	// Add closed order to performance log
	if err := sim.prfmLog.AddOrder(newClosedOrder); err != nil {
		return err
	}
	return nil
}

// Process simulates tick data going through our simulation pipeline
func (sim *Simulation) process(tick *instrument.Tick) error {

	// log.Println("Updating benchmark")
	if err := sim.updateBenchmark(*tick); err != nil {
		log.Fatal(err)
	}
	// log.Println("Checking if possible buy order")
	newOrder, txAmount, err := sim.buyOrderCheck(*tick)
	if err != nil && err != trading.ErrOrderNotValid {
		log.Fatal(err)
	}
	if newOrder != nil {
		// log.Println("Adding to portfolio")
		if err := sim.addToPortfolio(newOrder, txAmount); err != nil {
			log.Fatal(err)
		}
	}
	if err := sim.sellOrderCheck(tick); err != nil {
		log.Fatal(err)
	}
	return nil
}

type colConfig struct {
	tick, bid, bidSz, ask, askSz, tStamp uint8
	filedate                             time.Time
	timeUnit                             string
}

type worker struct {
	dataChan chan []string
	colCfg   colConfig
}

func newWorker(cols colConfig) *worker {
	worker := &worker{
		colCfg: cols,
	}
	return worker
}

func (worker *worker) run(outChan chan<- *instrument.Tick, r io.ReadSeeker) {
	var lineCount int
	done := make(chan struct{}, 2)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCount++
	}
	r.Seek(0, 0)

	worker.dataChan = make(chan []string, lineCount)
	go worker.send(outChan, done)
	go worker.produce(done, r)

	<-done
	<-done
}

func (worker *worker) send(outChan chan<- *instrument.Tick, done chan struct{}) {
	for {
		data, ok := <-worker.dataChan
		if !ok {
			if len(worker.dataChan) == 0 {
				close(outChan)
				break
			}
		}
		tick, err := worker.consume(data)
		if tick != nil && err == nil {
			outChan <- tick
		}
	}
	done <- struct{}{}
}

// 3 by 2 feet
func (worker *worker) produce(done chan struct{}, r io.ReadSeeker) {
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)

	scanner.Scan() // for headers...
	for scanner.Scan() {
		line := scanner.Text()

		// Check to see if error has been thrown or
		if err := scanner.Err(); err != nil {
			if err == io.EOF {
				break
			} else {
				log.Fatalln(err)
			}
		}
		record := strings.Split(line, "|")
		if len(record) > 4 {
			worker.dataChan <- record
		}
	}
	close(worker.dataChan)
	log.Println("done reading from file")
	done <- struct{}{}
}

func (worker *worker) consume(record []string) (*instrument.Tick, error) {
	var loadErr, parseErr error
	var tick *instrument.Tick

	tick = new(instrument.Tick)
	tick.Ticker = record[worker.colCfg.tick]

	bid, bidErr := strconv.ParseFloat(record[worker.colCfg.bid], 64)
	if bid == 0 {
		return tick, errors.New("bid Price could not be parsed")
	}
	if bidErr != nil {
		loadErr = errors.New("bid Price could not be parsed")
	}
	tick.Bid = utils.FloatAmount(bid)

	bidSz, bidSzErr := strconv.ParseFloat(record[worker.colCfg.bidSz], 64)
	if bidSzErr != nil {
		loadErr = errors.New("bid Size could not be parsed")
	}
	tick.BidSize = utils.Amount(bidSz)

	ask, askErr := strconv.ParseFloat(record[worker.colCfg.ask], 64)
	if ask == 0 {
		return nil, askErr
	}
	if askErr != nil {
		loadErr = errors.New("ask Price could not be parsed")
	}
	tick.Ask = utils.FloatAmount(ask)

	askSz, askSzErr := strconv.ParseFloat(record[worker.colCfg.askSz], 64)
	if askSzErr != nil {
		loadErr = errors.New("ask Size could not be parsed")
	}
	tick.AskSize = utils.Amount(askSz)

	tickDuration, timeErr := time.ParseDuration(record[worker.colCfg.tStamp] + worker.colCfg.timeUnit)

	if timeErr != nil {
		loadErr = timeErr
	}
	tick.Timestamp = worker.colCfg.filedate.Add(tickDuration)

	if parseErr != nil {
		return tick, parseErr
	}
	if loadErr != nil {
		log.Fatal("record could not be loaded")
	}
	return tick, nil
}
