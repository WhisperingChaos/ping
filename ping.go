package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// An alternative to "Synchronization queues in Golang" (https://medium.com/golangspec/synchronization-queues-in-golang-554f8e3a31a4)
// Detailed explaination/comparison ()
var latencyTot int64
var playerTot int64
var Games int64
var tableSimultaneous int64

func main() {
	memMonitor(500 * time.Millisecond)
	traceGen(120*time.Second, 500*time.Millisecond)
	testerCnt := 1000000
	// buffered channels large enough to accomidate
	// all players ensure FIFO. also, support greater
	// number of players as queue of interfaces
	// and the structs they reference consume much
	// less memory than a queue of blocked goroutines
	testers := make(chan play, testerCnt)
	go playerGen("Tester", testers, testerCnt)
	progrmCnt := 1000000
	progrms := make(chan play, progrmCnt)
	go playerGen("Programmer", progrms, progrmCnt)
	for i := 0; i < 1; i++ {
		go playersPair(testers, progrms)
	}
	select {}
}

func playerGen(role string, p chan play, max int) {
	var startWork sync.WaitGroup
	for i := 0; i < max; i++ {
		playerCreate(role+"_"+strconv.Itoa(i+1), p, &startWork)
		startWork.Wait()
	}
}

func playersPair(pque1 <-chan play, pque2 <-chan play) {
	table := make(chan []play)
	for {
		// fmt.Println()
		pque1sel := pque1
		pque2sel := pque2
		var p1 play
		select {
		case p1 = <-pque1sel:
			pque1sel = nil
		case p1 = <-pque2sel:
			pque2sel = nil
		}
		var p2 play
		select {
		case p2 = <-pque1sel:
			pque1sel = nil
		case p2 = <-pque2sel:
			pque2sel = nil
		}
		pair := make([]play, 2)
		pair[0] = p1
		pair[1] = p2
		select {
		case table <- pair:
		default:
			go func(table <-chan []play) {
				newTot := atomic.AddInt64(&tableSimultaneous, 1)
				if newTot > 400000 {
					atomic.AddInt64(&tableSimultaneous, -1)
					return
				}
				selfDestruct := time.NewTicker(100 * time.Millisecond)
				termTable := true
				for {
					select {
					case pair := <-table:
						atomic.AddInt64(&Games, 1)
						atomic.AddInt64(&playerTot, 2)
						var playing sync.WaitGroup
						playing.Add(2)
						pair[0].play(&playing)
						pair[1].play(&playing)
						playing.Wait()
						termTable = false
					case <-selfDestruct.C:
						if termTable {
							selfDestruct.Stop()
							atomic.AddInt64(&tableSimultaneous, -1)
							return
						}
						termTable = true
					}
				}
			}(table)
			table <- pair
		}
	}
}

type play interface {
	play(playing *sync.WaitGroup)
}

type player struct {
	role    string
	que     chan play
	latency time.Time
}

func (p player) latencyPrnt() {
	lat := time.Now().Sub(p.latency)
	atomic.AddInt64(&latencyTot, int64(lat))
	if lat > 10*time.Second {
		fmt.Println("latency: " + lat.String())
	}
}
func playerCreate(role string, que chan play, startWork *sync.WaitGroup) {
	p := player{
		role: role,
		que:  que,
	}
	startWork.Add(1)
	go func(p player, startWork *sync.WaitGroup) {
		// start work for this player before starting another player
		// enforces runtime ordering that might otherwise unfairly reorder goroutines
		// whose timers expire at or near the same instant.  Note - due to random duration
		// a more recent player's work timer may expire nearly before, nearly
		// after, or at the same time when compared to a previous player that's
		// already working.
		startWork.Done()
		p.work()
	}(p, startWork)
}
func (p player) play(playing *sync.WaitGroup) {
	p.latencyPrnt()
	var start sync.WaitGroup
	start.Add(1)
	go func(p player, start *sync.WaitGroup, playing *sync.WaitGroup) {
		//		fmt.Printf("%s starts\n", p.role)
		// ensure current player starts playing before a subsequent player. ("Happens Before")
		start.Done()
		// must play at least one millisecond but no more than 2 seconds.
		time.Sleep(time.Duration(rand.Intn(1999)+1) * time.Millisecond)
		//		fmt.Printf("%s ends\n", p.role)
		playing.Done()
		go p.work()
	}(p, &start, playing)
	start.Wait()
}
func (p player) work() {
	// must work at least one millisecond but no more than 10 seconds.
	time.Sleep(time.Duration(rand.Intn(9999)+1) * time.Millisecond)
	p.latency = time.Now()
	p.que <- p
}

// below not part of the solution. used to generate snapshot of memory allocated and being used.
func memMonitor(freq time.Duration) {
	var started sync.WaitGroup
	started.Add(1)
	go func(started *sync.WaitGroup) {
		started.Done()
		for {
			time.Sleep(freq)
			memUsage()
		}
	}(&started)
	started.Wait()
}
func memUsage() {
	// adapted from https://golangcode.com/print-the-current-memory-usage/
	var m runtime.MemStats
	// run GC so memory figures accurately represent inuse memory amounts.
	runtime.GC()
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("memory: Alloc = %s", byteScale(m.Alloc))
	fmt.Printf("\tTotalAlloc = %s", byteScale(m.TotalAlloc))
	fmt.Printf("\tStack Used = %s", byteScale(m.StackInuse))
	fmt.Printf("\tSys = %s", byteScale(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}
func byteScale(b uint64) string {
	scale := " B"
	switch {
	case b >= 1024*1024:
		b = b / (1024 * 1024)
		scale = " MB"
	case b >= 1024:
		b = b / 1024
		scale = " KB"
	}
	return fmt.Sprintf("%v", b) + scale
}
func traceGen(start time.Duration, interval time.Duration) {
	var started sync.WaitGroup
	started.Add(1)
	go func(start time.Duration, interval time.Duration, started *sync.WaitGroup) {
		started.Done()
		f, err := os.Create("trace.out")
		if err != nil {
			panic(err)
		}
		defer f.Close()
		time.Sleep(start)
		//		err = trace.Start(f)
		if err != nil {
			panic(err)
		}
		atomic.StoreInt64(&Games, 0)
		time.Sleep(interval)
		//		trace.Stop()
		latAvg := latencyTot / playerTot
		latAvgDur := time.Duration(latAvg)
		fmt.Printf("memory: game total: %v, overlapping: %v, Avg Latency: %s\n", Games, tableSimultaneous, latAvgDur.String())
		os.Exit(0)
	}(start, interval, &started)
	started.Wait()
}
