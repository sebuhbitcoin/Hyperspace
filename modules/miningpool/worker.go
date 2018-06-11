package pool

import (
	"path/filepath"
	"time"

	"github.com/sasha-s/go-deadlock"

	"github.com/NebulousLabs/Sia/persist"
)

type ShareRecord struct {
	jobID             uint32
	extraNonce2       string
	ntime             string
	nonce             string
	nonce1            string
}

type WorkerRecord struct {
	name              string
	workerID          int64

	shareDifficulty   float64

	blocksFound       uint64
	parent            *Client
}

//
// A Worker is an instance of one miner.  A Client often represents a user and the
// worker represents a single miner.  There is a one to many client worker relationship
//
type Worker struct {
	mu deadlock.RWMutex
	wr WorkerRecord
	s  *Session
	// utility
	log *persist.Logger
}

func newWorker(c *Client, name string, s *Session) (*Worker, error) {
	p := c.Pool()
	// id := p.newStratumID()
	w := &Worker{
		wr: WorkerRecord{
			// workerID: id(),
			name:     name,
			parent:   c,
		},
		s: s,
	}

	var err error

	// Create the perist directory if it does not yet exist.
	dirname := filepath.Join(p.persistDir, "clients", c.Name())
	err = p.dependencies.mkdirAll(dirname, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger, and set up the stop call that will close the
	// logger.
	w.log, err = p.dependencies.newLogger(filepath.Join(dirname, name+".log"))

	return w, err
}

func (w *Worker) printID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return ssPrintID(w.wr.workerID)
}

func (w *Worker) Name() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.wr.name
}

func (w *Worker) SetName(n string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wr.name = n
}

func (w *Worker) Parent() *Client {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.wr.parent
}

func (w *Worker) SetParent(p *Client) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wr.parent = p
}

func (w *Worker) Session() *Session {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.s
}

func (w *Worker) SetSession(s *Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.s = s
}

func (w *Worker) IncrementShares(sessionDifficulty float64, reward float64) {
	p := w.s.Client.pool
	cbid := p.cs.CurrentBlock().ID()
	blockTarget, _ := p.cs.ChildTarget(cbid)
	blockDifficulty, _ := blockTarget.Difficulty().Uint64()

	sessionTarget, _ := difficultyToTarget(sessionDifficulty)
	siaSessionDifficulty, _ := sessionTarget.Difficulty().Uint64()
	shareRatio := caculateRewardRatio(sessionTarget.Difficulty().Big(), blockTarget.Difficulty().Big())
	shareReward := shareRatio * reward
	// w.log.Printf("shareRatio: %f, shareReward: %f", shareRatio, shareReward)

	share := &Share{
		userid:          w.Parent().cr.clientID,
		workerid:        w.wr.workerID,
	    height:          int64(p.cs.Height())+1,
		valid:           true,
		difficulty:      sessionDifficulty,
		shareDifficulty: float64(siaSessionDifficulty),
		reward:          reward,
		blockDifficulty: float64(blockDifficulty),
		shareReward:     shareReward,
		time: time.Now(),
	}

	w.s.Shift().IncrementShares(share)
}

func (w *Worker) IncrementInvalidShares() {
	w.s.Shift().IncrementInvalid()
}

func (w *Worker) SetLastShareTime(t time.Time) {
	w.s.Shift().SetLastShareTime(t)
}

func (w *Worker) LastShareTime() time.Time {
	return w.s.Shift().LastShareTime()
	// unixTime := w.getUint64Field("LastShareTime")
	// return time.Unix(int64(unixTime), 0)
}

func (w *Worker) BlocksFound() uint64 {
	return w.wr.blocksFound
}

func (w *Worker) IncrementBlocksFound() {
	w.wr.blocksFound++
	w.updateWorkerRecord()
}

// func (w *Worker) CumulativeDifficulty() float64 {
// 	return w.s.Shift().CumulativeDifficulty()
// 	// return w.getFloatField("CumulativeDifficulty")
// }

// CurrentDifficulty returns the average difficulty of all instances of this worker
func (w *Worker) CurrentDifficulty() float64 {
	pool := w.wr.parent.Pool()
	d := pool.dispatcher
	d.mu.Lock()
	defer d.mu.Unlock()
	workerCount := uint64(0)
	currentDiff := float64(0.0)
	for _, h := range d.handlers {
		if h.s.Client != nil && h.s.Client.Name() == w.Parent().Name() && h.s.CurrentWorker.Name() == w.Name() {
			currentDiff += h.s.CurrentDifficulty()
			workerCount++
		}
	}
	if workerCount == 0 {
		return 0.0
	}
	return currentDiff / float64(workerCount)
}

func (w *Worker) Online() bool {
	return w.s != nil
}
