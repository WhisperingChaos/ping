package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// An alternative to "Synchronization queues in Golang" (https://medium.com/golangspec/synchronization-queues-in-golang-554f8e3a31a4)
// Detailed explaination/comparison ()
func main() {
	testerCnt := 5
	// Allocate buffered channels large enough to accommodate
	// all players ensuring FIFO. Supports greater
	// number of players as queue of interfaces
	// and the structs they reference consume much
	// less memory than a queue of blocked goroutines in
	// situations when latency, due to waiting for
	// matching ready player and then once paired, waiting
	// for others to finish playing, exceeds certain threshold.
	testers := make(chan play, testerCnt)
	go playerGen("Tester", testers, testerCnt)
	progrmCnt := 10
	progrms := make(chan play, progrmCnt)
	go playerGen("Programmer", progrms, progrmCnt)
	go playersPair(testers, progrms)
	select {}
}

func playerGen(role string, p chan play, max int) {
	var startWork sync.WaitGroup
	for i := 0; i < max; i++ {
		p := player{
			role: role + "_" + strconv.Itoa(i+1),
			que:  p,
		}
		p.work(&startWork)
		startWork.Wait()
	}
}

func playersPair(pque1 <-chan play, pque2 <-chan play) {
	for {
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
		fmt.Println()
		var playing sync.WaitGroup
		p1.play(&playing)
		p2.play(&playing)
		playing.Wait()
	}
}

type play interface {
	play(playing *sync.WaitGroup)
}

type player struct {
	role string
	que  chan play
}

func (p player) play(playing *sync.WaitGroup) {
	playing.Add(1)
	go func(p player, playing *sync.WaitGroup) {
		fmt.Printf("%s starts\n", p.role)
		// must play at least one millisecond but no more than 2 seconds.
		time.Sleep(time.Duration(rand.Intn(1999)+1) * time.Millisecond)
		fmt.Printf("%s ends\n", p.role)
		playing.Done()
		var start sync.WaitGroup
		p.work(&start)
		start.Wait()
	}(p, playing)
}
func (p player) work(start *sync.WaitGroup) {
	start.Add(1)
	go func(p player, start *sync.WaitGroup) {
		// Start work for this player before starting another player ("Happpens Before").
		// Immediately executes this goroutine so its termination accurately
		// reflects the moment an individual decides to tansition from working to playing.
		// This behavior helps enforce fairness in situations involving other players
		// whose work timers expire at descernibly near the same instance as this one.
		// It's necessary since the runtime doesn't guarantee even executing a goroutine,
		// which would allow other players to cut in front of the current one.
		//
		// Must use same waitgroup to enforce "Happens Before" for
		// particular role and queue.
		start.Done()
		// must work at least one millisecond but no more than 10 seconds.
		time.Sleep(time.Duration(rand.Intn(9999)+1) * time.Millisecond)
		p.que <- p
	}(p, start)
}
