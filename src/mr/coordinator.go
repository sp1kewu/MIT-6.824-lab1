package mr

import "log"
import "net"
import "os"
import "net/rpc"
import "net/http"
//
import "time"
import "sync"
import "fmt"


type Task struct{
	TaskID int
	TaskType string // "map", "reduce", "wait", "exit"
	Filename string
	State string // "idle", "in-progress", "done"
	startTime time.Time
}

type Coordinator struct {
	// Your definitions here.
	Tasks []Task
	nReduce int
	nMap int
	phase string // "map", "reduce", "done"
	mutex sync.Mutex//互斥锁
}

// Your code here -- RPC handlers for the worker to call.

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

//coordinator assign task to worker
func (c *Coordinator) AssignTask(args *AskTaskArgs, reply *AskTaskReply) error{
	//上锁
	c.mutex.Lock()
	defer c.mutex.Unlock()
	//phase check
	if c.phase=="map"{
		for i:=0;i<len(c.Tasks);i++{
			if c.Tasks[i].State=="idle"&&c.Tasks[i].TaskType=="map"{
				reply.TaskType="map"
				c.Tasks[i].State="in-progress"
				c.Tasks[i].startTime=time.Now()
				reply.TaskID=c.Tasks[i].TaskID
				reply.NReduce=c.nReduce
				reply.NMap=c.nMap
				reply.Filename=c.Tasks[i].Filename
				return nil
			}
		}
	}else if c.phase=="reduce"{ 
		for i:=0;i<len(c.Tasks);i++{
			if c.Tasks[i].State=="idle"&&c.Tasks[i].TaskType=="reduce"{
				reply.TaskType="reduce"
				c.Tasks[i].State="in-progress"
				c.Tasks[i].startTime=time.Now()
				reply.TaskID=c.Tasks[i].TaskID
				reply.NReduce=c.nReduce
				reply.NMap=c.nMap
				return nil
			}
		}
	}else if c.phase=="done"{
		reply.TaskType="exit"
		return nil
	}
	//没有task在idle，等待
	reply.TaskType="wait"
	return nil
}

//coordinator receive report from worker
func (c *Coordinator) ReportTask(args *ReportTaskArgs, reply *ReportTaskReply) error{
	//上锁
	c.mutex.Lock()
	defer c.mutex.Unlock()

	//mark task as done
	for i:=0;i<len(c.Tasks);i++{
		if c.Tasks[i].TaskID==args.TaskID{
			c.Tasks[i].State="done"
		}
	}

	//检查当前phase是否都完成了
	allDone:=true
	for i:=0;i<len(c.Tasks);i++{
		if c.Tasks[i].TaskType==c.phase&&c.Tasks[i].State!="done"{
			allDone=false
			break
		}
	}

	//切换phase
	if allDone{
		if c.phase=="map"{
			//debug
			fmt.Printf("All map tasks done. Switch to reduce phase.\n")
			c.phase="reduce"
			//initialize reduce tasks
			c.Tasks=nil//清空Tasks列表
			for i:=0;i<c.nReduce;i++{
				task:=Task{TaskID: i, TaskType: "reduce", State: "idle"}
				c.Tasks=append(c.Tasks, task)
			}
		}else if c.phase=="reduce"{
			//debug
			fmt.Printf("All reduce tasks done. Job completed.\n")
			c.phase="done"
		}
	}
	return nil
}

//检测worker超时(10s)
func (c *Coordinator) DetectTimeout(){
	for{
		time.Sleep(time.Second)
		c.mutex.Lock()
		if c.phase=="done"{
				c.mutex.Unlock()
				return
		}
		for i:=0;i<len(c.Tasks);i++{
			if c.Tasks[i].State=="in-progress"&&time.Since(c.Tasks[i].startTime)>10*time.Second{
				//debug
				fmt.Printf("[Timeout] Task %d (%s) reset to idle\n", c.Tasks[i].TaskID, c.Tasks[i].TaskType)
				c.Tasks[i].State="idle"
			}
		}
		c.mutex.Unlock()
	}
}

//
// start a thread that listens for RPCs from worker.go
//
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
//
func (c *Coordinator) Done() bool {
	ret := false

	// Your code here.
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.phase=="done"{
		ret=true
	}

	return ret
}

//
// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{}

	// Your code here.

	//为map文件创建任务
	for i,f:=range files{
		task:= Task{TaskID: i, TaskType: "map", Filename: f, State: "idle"}
		c.Tasks=append(c.Tasks, task)
	}

	c.nMap=len(files)
	c.nReduce=nReduce
	c.phase="map"

	go c.DetectTimeout()//超时检测
	c.server()
	return &c
}
