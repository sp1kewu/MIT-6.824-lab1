# MIT 6.824 Lab 1：单机多进程 MapReduce 词频统计

本仓库为我在学习 MIT 6.824 课程过程中完成的 **Lab 1：MapReduce** 实验代码与说明。
实验目标是在 **单机多进程环境** 下，使用 Go 语言实现一个简化版的 MapReduce 框架，完成 **词频统计（word count）** 等任务，并通过官方测试脚本。

---

## 1. 实验目标与功能概述

**实验目标：**

* 理解 **MapReduce** 的基本计算模型（Map / Reduce 两阶段）。
* 学会使用 Go 语言实现简单的 **任务调度系统**。
* 使用 **RPC（net/rpc）** 完成进程间通信（Coordinator ↔ Worker）。
* 在单机上实现多个 Worker 进程并行执行，完成词频统计。
* 认识任务状态管理、超时重试等基本容错机制。

**实现功能：**

* 支持多个 Worker 进程并行执行 Map / Reduce 任务。
* Coordinator 负责：

  * 创建、分配并管理 Map / Reduce 任务；
  * 维护任务状态：`idle` / `in-progress` / `done`；
  * Map 阶段结束后切换至 Reduce 阶段，直至作业完成；
  * 定期检测任务超时，重置并重新分配（模拟 worker 崩溃场景）。
* Worker 负责：

  * 向 Coordinator 请求任务；
  * 执行 `mapf` / `reducef` 函数；
  * 写入中间文件 `mr-MapID-ReduceID`；
  * 生成最终输出文件 `mr-out-X`；
  * 完成后通过 RPC 上报任务状态。

---

## 2. 代码结构说明

以官方 6.824 仓库结构为基础，本实验主要涉及以下文件（只列出与 Lab 1 相关部分）：

```text
src/
  mr/
    coordinator.go   # Coordinator 实现：任务调度、RPC 处理、超时检测
    worker.go        # Worker 实现：DoMap / DoReduce / RPC 客户端
    rpc.go           # RPC 参数与返回结构体定义，coordinatorSock() 等
  main/
    mrcoordinator.go # 官方提供的 coordinator 入口，不修改
    mrworker.go      # 官方提供的 worker 入口，不修改
```

其中：

* `coordinator.go`：实现 Coordinator 结构体及其方法（`AssignTask`、`ReportTask`、`Done`、超时检测等）。
* `worker.go`：实现 Worker 主循环、`DoMap`、`DoReduce`、`AskTask`、`ReportTask`。
* `rpc.go`：定义 Coordinator 与 Worker 之间的 RPC 接口结构体（请求 / 回复）。

---

## 3. 任务分析与整体设计

### 3.1 任务分析

实验任务本质上是实现一个“**单机多进程版的 MapReduce 框架**”，并在此框架上完成词频统计（word count）和倒排索引（indexer）等操作。

* **输入**：若干文本文件（如 `pg-*.txt`）。
* **Map 阶段**：把文件内容拆分为 `(word, 1)` 形式的键值对。
* **Reduce 阶段**：对所有相同 `word` 的值进行累加，得到该词在所有文件中的总出现次数。
* **输出**：多个 `mr-out-X` 文件，最终再合并作为结果。

在整个过程中，Coordinator 作为“调度中心”，Worker 作为“执行单元”，二者通过 RPC 交互完成任务分发与结果回报。

### 3.2 整体交互流程

简化流程如下：

1. 启动 Coordinator，初始化 Map 任务列表，进入 `"map"` 阶段。
2. 启动多个 Worker 进程。
3. Worker 循环通过 RPC 调用 `AssignTask` 请求任务。
4. Coordinator 根据当前阶段和任务状态分配相应的 Map / Reduce 任务。
5. Worker 执行 `DoMap` 或 `DoReduce`，完成后调用 `ReportTask` 进行汇报。
6. Coordinator 标记任务完成，所有 Map 任务完成后切换至 Reduce 阶段。
7. 所有 Reduce 任务完成后，Coordinator 将阶段设为 `"done"`，Worker 收到 `"exit"` 后退出。

---

## 4. Coordinator 设计与实现

### 4.1 数据结构

```go
type Task struct {
    TaskID    int
    TaskType  string    // "map" 或 "reduce"
    Filename  string    // Map 阶段使用的输入文件名
    State     string    // "idle", "in-progress", "done"
    startTime time.Time // 任务开始时间，用于超时检测
}

type Coordinator struct {
    Tasks   []Task      // 当前阶段的任务列表
    nReduce int
    nMap    int
    phase   string      // "map", "reduce", "done"
    mutex   sync.Mutex  // 用于保护共享状态
}
```

* `Tasks`：在 Map 阶段存储所有 Map 任务；在 Reduce 阶段重新初始化为 Reduce 任务。
* `phase`：当前所处的阶段；用于决定向 Worker 分配什么类型的任务。

### 4.2 任务分配（AssignTask）

```go
func (c *Coordinator) AssignTask(args *AskTaskArgs, reply *AskTaskReply) error {
    c.mutex.Lock()
    defer c.mutex.Unlock()

    if c.phase == "map" {
        for i := range c.Tasks {
            if c.Tasks[i].TaskType == "map" && c.Tasks[i].State == "idle" {
                c.Tasks[i].State = "in-progress"
                c.Tasks[i].startTime = time.Now()
                reply.TaskType = "map"
                reply.TaskID = c.Tasks[i].TaskID
                reply.Filename = c.Tasks[i].Filename
                reply.NReduce = c.nReduce
                reply.NMap = c.nMap
                return nil
            }
        }
    } else if c.phase == "reduce" {
        for i := range c.Tasks {
            if c.Tasks[i].TaskType == "reduce" && c.Tasks[i].State == "idle" {
                c.Tasks[i].State = "in-progress"
                c.Tasks[i].startTime = time.Now()
                reply.TaskType = "reduce"
                reply.TaskID = c.Tasks[i].TaskID
                reply.NReduce = c.nReduce
                reply.NMap = c.nMap
                return nil
            }
        }
    } else if c.phase == "done" {
        reply.TaskType = "exit"
        return nil
    }

    // 当前阶段任务都在执行中，返回等待
    reply.TaskType = "wait"
    return nil
}
```

要点：

* 使用互斥锁避免多个 Worker 同时修改任务状态。
* 若没有空闲任务但还未完成，则让 Worker 进入等待状态（`wait`）。
* 当阶段为 `done` 时，返回 `exit` 通知 Worker 退出。

### 4.3 任务完成上报（ReportTask）

Worker 执行完任务后调用：

```go
func (c *Coordinator) ReportTask(args *ReportTaskArgs, reply *ReportTaskReply) error {
    c.mutex.Lock()
    defer c.mutex.Unlock()

    // 标记对应任务为 done
    for i := range c.Tasks {
        if c.Tasks[i].TaskID == args.TaskID {
            c.Tasks[i].State = "done"
        }
    }

    // 判断当前阶段的任务是否全部完成
    allDone := true
    for i := range c.Tasks {
        if c.Tasks[i].TaskType == c.phase && c.Tasks[i].State != "done" {
            allDone = false
            break
        }
    }

    // 阶段切换
    if allDone {
        if c.phase == "map" {
            // 切换到 reduce 阶段
            c.phase = "reduce"
            c.Tasks = nil
            for i := 0; i < c.nReduce; i++ {
                c.Tasks = append(c.Tasks, Task{
                    TaskID:   i,
                    TaskType: "reduce",
                    State:    "idle",
                })
            }
        } else if c.phase == "reduce" {
            c.phase = "done"
        }
    }

    return nil
}
```

### 4.4 超时检测（任务容错）

为了应对 Worker 崩溃或长时间卡住的情况，Coordinator 启动一个独立的 goroutine 定期检查任务是否超时：

```go
func (c *Coordinator) DetectTimeout() {
    for {
        time.Sleep(time.Second)
        c.mutex.Lock()
        if c.phase == "done" {
            c.mutex.Unlock()
            return
        }
        for i := range c.Tasks {
            if c.Tasks[i].State == "in-progress" &&
               time.Since(c.Tasks[i].startTime) > 10*time.Second {
                // 超时任务重置为 idle，等待重新分配
                c.Tasks[i].State = "idle"
            }
        }
        c.mutex.Unlock()
    }
}
```

---

## 5. Worker 设计与实现

### 5.1 Worker 主循环

```go
func Worker(
    mapf func(string, string) []KeyValue,
    reducef func(string, []string) string,
) {
    workerID := os.Getpid() // 用进程 PID 作为 Worker ID

    for {
        task := AskTask(workerID)
        switch task.TaskType {
        case "map":
            DoMap(task, mapf)
            ReportTask(task.TaskID, "map", workerID)
        case "reduce":
            DoReduce(task, reducef)
            ReportTask(task.TaskID, "reduce", workerID)
        case "wait":
            time.Sleep(time.Second)
        case "exit":
            return
        }
    }
}
```

### 5.2 Map 阶段实现（DoMap）

```go
func DoMap(task AskTaskReply, mapf func(string, string) []KeyValue) {
    content, _ := os.ReadFile(task.Filename)
    kva := mapf(task.Filename, string(content))

    // 按 reduceID 分桶
    buckets := make([][]KeyValue, task.NReduce)
    for _, kv := range kva {
        r := ihash(kv.Key) % task.NReduce
        buckets[r] = append(buckets[r], kv)
    }

    // 每个桶写一个中间文件：mr-MapID-ReduceID
    for r, kvs := range buckets {
        filename := fmt.Sprintf("mr-%d-%d", task.TaskID, r)
        file, _ := os.Create(filename)
        enc := json.NewEncoder(file)
        for _, kv := range kvs {
            enc.Encode(&kv)
        }
        file.Close()
    }
}
```

### 5.3 Reduce 阶段实现（DoReduce）

```go
func DoReduce(task AskTaskReply, reducef func(string, []string) string) {
    kvs := make(map[string][]string)

    // 读取所有归属于该 reduceID 的中间文件
    for mapID := 0; mapID < task.NMap; mapID++ {
        filename := fmt.Sprintf("mr-%d-%d", mapID, task.TaskID)
        file, err := os.Open(filename)
        if err != nil {
            continue
        }
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

    // 对 key 排序，保证输出顺序稳定
    keys := make([]string, 0, len(kvs))
    for k := range kvs {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    // 输出结果到 mr-out-ReduceID
    outFile, _ := os.Create(fmt.Sprintf("mr-out-%d", task.TaskID))
    for _, k := range keys {
        result := reducef(k, kvs[k])
        fmt.Fprintf(outFile, "%v %v\n", k, result)
    }
    outFile.Close()
}
```

---

## 6. RPC 接口设计

在 `rpc.go` 中定义 RPC 请求与返回结构体：

```go
type AskTaskArgs struct {
    WorkerID int
}

type AskTaskReply struct {
    TaskType string // "map", "reduce", "wait", "exit"
    TaskID   int
    Filename string
    NReduce  int
    NMap     int
}

type ReportTaskArgs struct {
    WorkerID int
    TaskID   int
    TaskType string // "map" 或 "reduce"
}

type ReportTaskReply struct {}
```

Worker 端 RPC 封装：

```go
func AskTask(workerID int) AskTaskReply {
    args := AskTaskArgs{WorkerID: workerID}
    reply := AskTaskReply{}
    call("Coordinator.AssignTask", &args, &reply)
    return reply
}

func ReportTask(taskID int, taskType string, workerID int) ReportTaskReply {
    args := ReportTaskArgs{WorkerID: workerID, TaskID: taskID, TaskType: taskType}
    reply := ReportTaskReply{}
    call("Coordinator.ReportTask", &args, &reply)
    return reply
}
```

`call` 函数使用 `net/rpc` 与 `unix` domain socket 实现 RPC 调用。

---

## 7. 运行与测试

### 7.1 手动运行

在 6.824 源码目录下（或对应环境）：

```bash
# 启动 coordinator
go run main/mrcoordinator.go pg-*.txt

# 另开若干终端，启动多个 worker
go run main/mrworker.go wc.so
go run main/mrworker.go wc.so
# 根据需要再多开几个 worker
```

运行结束后，会在当前目录生成：

* 中间文件：`mr-MapID-ReduceID`
* 输出文件：`mr-out-X`

### 7.2 使用官方测试脚本

推荐使用课程提供的测试脚本：

```bash
bash test-mr.sh
```

通过修正文件覆盖、字段大小写、超时检测逻辑等问题后，最终测试结果为：

* `wc` 测试：PASS
* `indexer` 测试：PASS
* map 并行性测试：PASS
* reduce 并行性测试：PASS
* job count 测试：PASS
* early exit 测试：PASS
* crash 容错测试：PASS

<img width="1004" height="1174" alt="image" src="https://github.com/user-attachments/assets/12266979-19b1-42cf-8630-0bcb45ed67a5" />


---

## 8. 实验总结

在本次 Lab 1 中，我从零实现了一个简化版的单机 MapReduce 框架，过程中主要有以下收获：

* 对 Map / Reduce 的整体执行流程和责任划分有了直观理解；
* 实际动手实现了基于 RPC 的进程间通信，注意到结构体字段必须首字母大写才能被序列化；
* 通过任务状态与超时检测，理解了简单的容错机制设计；
* 通过修复中间文件被覆盖、输出格式不一致等 bug，锻炼了调试和问题定位能力；
  


