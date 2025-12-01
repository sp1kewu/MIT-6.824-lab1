package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.
//worker ask for task
type AskTaskArgs struct {
	WorkerID int
}
//coordinator reply task to worker
type AskTaskReply struct {
	TaskType   string // "map", "reduce","wait","exit"
	TaskID int
	Filename   string   // for map task
	NReduce    int      // for map task
	NMap       int      // for reduce task
}
//worker report task to coordinator
type ReportTaskArgs struct {
	WorkerID int
	TaskID   int
	TaskType string // "map", "reduce"
}
//null
type ReportTaskReply struct {
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}

