package mr

import "fmt"
import "log"
import "net/rpc"
import "hash/fnv"
//
import "os"
import "time"
import "encoding/json"
import "sort"


//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//Map功能实现函数
func DoMap(task AskTaskReply, mapf func(string, string) []KeyValue) {
    content, _ := os.ReadFile(task.Filename)
    kva := mapf(task.Filename, string(content))

    buckets := make([][]KeyValue, task.NReduce)
for _, kv := range kva {
    r := ihash(kv.Key) % task.NReduce
    buckets[r] = append(buckets[r], kv)
}

for r, kvs := range buckets {
    filename := fmt.Sprintf("mr-%d-%d", task.TaskID, r)
    file, err := os.Create(filename)
    if err != nil {
        log.Fatalf("cannot create %v", filename)
    }
    enc := json.NewEncoder(file)
    for _, kv := range kvs {
        err := enc.Encode(&kv)
        if err != nil {
            log.Fatalf("cannot encode kv: %v", err)
        }
    }
    file.Close()
}

}
//Reduce功能实现函数
func DoReduce(task AskTaskReply, reducef func(string, []string) string) {
    kvs := make(map[string][]string)

    for mapID := 0; mapID < task.NMap; mapID++ {
        filename := fmt.Sprintf("mr-%d-%d", mapID, task.TaskID)
        file, _ := os.Open(filename)
        dec := json.NewDecoder(file)
        for {
            var kv KeyValue
            if err := dec.Decode(&kv); err != nil {
                break
            }
            kvs[kv.Key] = append(kvs[kv.Key], kv.Value)
        }
        file.Close()
    }

    keys := make([]string, 0, len(kvs))
    for k := range kvs {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    outFile, _ := os.Create(fmt.Sprintf("mr-out-%d", task.TaskID))
    for _, k := range keys {
        result := reducef(k, kvs[k])
        fmt.Fprintf(outFile, "%v %v\n", k, result)
    }
    outFile.Close()
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
	worker_id := os.Getpid()//使用进程pid作为worker id
	//循环向coordinator请求任务，直到收到exit任务
	for{
		task:=AskTask(worker_id)
		if task.TaskType=="map"{
			DoMap(task, mapf)
			ReportTask(task.TaskID, "map", worker_id)
		}else if task.TaskType=="reduce"{
			DoReduce(task, reducef)
			ReportTask(task.TaskID, "reduce", worker_id)
		}else if task.TaskType=="wait"{
			time.Sleep(time.Second)
		}else if task.TaskType=="exit"{
			break
		}
	}
	// uncomment to send the Example RPC to the coordinator.
	// CallExample()

}

//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

//worker ask for task
func AskTask(worker_id int) AskTaskReply {
	args:= AskTaskArgs{WorkerID: worker_id}
	reply:= AskTaskReply{}
	call("Coordinator.AssignTask", &args, &reply)
	return reply
}

//worker report task to coordinator
func ReportTask(task_id int, task_type string, worker_id int)ReportTaskReply {
	args:= ReportTaskArgs{WorkerID: worker_id, TaskID: task_id, TaskType: task_type}
	reply:= ReportTaskReply{}
	call("Coordinator.ReportTask", &args, &reply)
	return reply
}