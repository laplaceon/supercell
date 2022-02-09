package main

import (
	"fmt"
	"gopkg.in/vmihailenco/msgpack.v2"
	zmq "github.com/pebbe/zmq4"
	"strings"
	"sync"
	"time"
)

var (
	supervisorIp string
	// Follows Mark Handley's supervisor-worker model - http://bit.ly/QSX5q8
	// The "bag" is the "sink" in this case
	sinkIp string
	numWorkers = 0
	supervisor *zmq.Socket
	worker *zmq.Socket
	tracker *zmq.Socket
	notifier *zmq.Socket
	supervisorLock *sync.Mutex
)

func WaitForWorkers(quit chan bool) {
ListenLoop:
	for {
		select {
			case <-quit:
				break ListenLoop
			default:
		}
		
		if id, _ := tracker.Recv(zmq.DONTWAIT); id != "" {
			tracker.Recv(0)
			message, _ := tracker.Recv(0)
			
			tracker.Send(id, zmq.SNDMORE)
			tracker.Send("", zmq.SNDMORE)
			tracker.Send("ok", 0)
			
			if message == "started" {
				numWorkers++
				fmt.Println("A worker joined!")
			}
		}
	}
}

func ListenForSupervisorFinish() {
	for {
		// notifier.Recv(0)
		message, _ := notifier.Recv(0)
		
		messageParts := strings.Split(message, " ")
		
		if messageParts[1] == "done" {
			wg.Done()
			break
		}
	}
}

func WaitForWorkersToFinish() {
	completed := 0
	for {
		id, _ := tracker.Recv(0)
		tracker.Recv(0)
		message, _ := tracker.Recv(0)
		
		// When completion signal is received, incremented the completed count
		// If completed count is equal to the number of workers, we're done
		if message == "finished" {
			fmt.Println("Received finish")
			
			// Send confirmation
			tracker.Send(id, zmq.SNDMORE)
			tracker.Send("", zmq.SNDMORE)
			tracker.Send("ok", 0)
			
			completed++
			if completed == numWorkers {
				break
			}
		}
	}
}

func WaitForWork() {
	for {
		work, _ := worker.RecvBytes(0)
		
		var job ScrapeJob
		err := msgpack.Unmarshal(work, &job)
		if err == nil {
			wg.Add(1)
			if job.Limited {
				lbQueue <- job
			} else {
				hbQueue <- job
			}
		}
	}
}

func SendWork(source string, link string, limited bool) {
	b, err := msgpack.Marshal(&ScrapeJob{Source: source, Link: link, Limited: limited})
	if err != nil {
		fmt.Println("Error packaging scrape job")
		return
	}

	supervisorLock.Lock()
	supervisor.SendBytes(b, 0)
	supervisorLock.Unlock()
}

func InitiateSupervisor() {
	// Distribute scrape commands through this socket
	supervisor, _ = zmq.NewSocket(zmq.PUSH)
	supervisor.SetLinger(500 * time.Millisecond)
	supervisor.Bind("tcp://*:5557")
	
	supervisorLock = &sync.Mutex{}
	
	// Receive start and done states through this socket
	tracker, _ = zmq.NewSocket(zmq.ROUTER)
	tracker.Bind("tcp://*:5558")
	
	// Send done signal to notify workers that the supervisor is done handing out work
	notifier, _ = zmq.NewSocket(zmq.PUB)
	notifier.Bind("tcp://*:5559")
}

func InitiateWorker() {
	// Receive scrape commands through this socket
	worker, _ = zmq.NewSocket(zmq.PULL)
	worker.SetLinger(750 * time.Millisecond)
	worker.Connect("tcp://" + supervisorIp + ":5557")

	// Send start and done states through this socket
	tracker, _ = zmq.NewSocket(zmq.REQ)
	tracker.Connect("tcp://" + supervisorIp + ":5558")
	
	// Receive done signal through this socket
	notifier, _ = zmq.NewSocket(zmq.SUB)
	notifier.Connect("tcp://" + supervisorIp + ":5559")

	// Filter out other messages
	notifier.SetSubscribe("notifier ")
}