// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package etcdload

import (
    "strconv"
    "strings"
    "os"
    "fmt"
    "github.com/coreos/go-etcd/etcd"
    "crypto/rand"
    "time"
    "sync"
    "os/exec"
    "math/big"
    "log"
    "code.google.com/p/gcfg"
    "code.google.com/p/go.crypto/ssh"
    "io/ioutil"
    "bytes"
    "flag"
)

/*
Declarations :::: 
    actions   : for passing otherfunctions as arguments to handler function
    operation : which operation to perform
    keycount  : number of keys to be added, retrieved, deleted or updated
    threads   : number of total threads
    pct       : each entry represents percentage of values lying in (value_range[i],value_range[i+1]) 
*/

type actions func(int,int)

var (
    wg sync.WaitGroup
    operation, pidetcd, pidetcd_s, conf_file string
    keycount, operation_count, threads, etcdmem_s, etcdmem_e int
    pct, pct_count, value_range []int
    client *etcd.Client
    start time.Time
    f *os.File
    err error
    key ssh.Signer
    config *ssh.ClientConfig
    ssh_client *ssh.Client
    session *ssh.Session
    mem_flag bool
    results chan *result
)

//flag variables
var (
    fhost, fport, foperation, flog_file, fcfg_file *string
    fkeycount, foperation_count *int
    fremote_flag, fmem_flag, fhelp *bool
)

 func init() {
    // All the defaults are from the etcd_load.cfg default
    fhelp = flag.Bool("help", false, "shows how to use flags")    
    fhost = flag.String("h", "null", "etcd instance address."+
                        "Default=127.0.0.1 from config file")
    fport = flag.String("p", "null", 
                        "port on which etcd is running. Defalt=4001")
    foperation = flag.String("o", "null", 
                        "operation - create/delete/get/update. Default:create")
    fkeycount = flag.Int("k",-1,"number of keys involved in operation,"+
                        "useful for create. Default:100")
    foperation_count = flag.Int("oc",-1,"number of operations to be performed,"+
                        " Default:200")
    flog_file = flag.String("log","null", "logfile name, default : log")
    fremote_flag = flag.Bool("remote",false," Must be set true if etcd "+
                        "instance is remote. Default=false")
    fmem_flag = flag.Bool("mem",false,"When true, memory info is shown."+
                        " Default=false")
    fcfg_file = flag.String("c","null","Input the cfg file. Required")
 }

func main() {
    //parsing commandline flags
    flag.Parse()

    // Configuration Structure
    cfg := struct {
        Section_Args struct {
            Etcdhost string
            Etcdport string
            Operation string
            Keycount string
            Operation_Count string
            Log_File string
            Threads int
            Pct string
            Value_Range string
            Remote_Flag bool
            Ssh_Port string
            Remote_Host_User string
        }
    }{}
    
    // Config File
    if *fhelp {
        flag.Usage()
        return
    }
    if *fcfg_file != "null"{
        conf_file = *fcfg_file
    } else
    {
        flag.Usage()
        fmt.Println("Please input the cfg file")
        return
    }
    err := gcfg.ReadFileInto(&cfg, conf_file)
    if err != nil {
        log.Fatalf("Failed to parse gcfg data: %s", err)
    }


    // Reading from config file
    ////////////////////////////////////////////////////////
    etcdhost := cfg.Section_Args.Etcdhost
    etcdport := cfg.Section_Args.Etcdport
    operation = cfg.Section_Args.Operation
    keycount = int(toInt(cfg.Section_Args.Keycount,10,64))
    operation_count = int(toInt(cfg.Section_Args.Operation_Count,10,64))
    log_file := cfg.Section_Args.Log_File

    // The Distribution of keys
    percents := cfg.Section_Args.Pct
    temp := strings.Split(percents,",")
    pct = make([]int,len(temp))
    pct_count = make([]int,len(temp))
    for i:=0;i<len(temp);i++ {
        pct[i] = int(toInt(temp[i],10,64))
        pct_count[i] = pct[i] * keycount / 100
    }
    // Percentage distribution of key-values
    value_r := cfg.Section_Args.Value_Range
    temp = strings.Split(value_r,",")
    value_range = make([]int,len(temp))
    for i:=0;i<len(temp);i++ {
        value_range[i] = int(toInt(temp[i],10,64))
    }
    //Maximum threads
    threads = cfg.Section_Args.Threads

    remote_flag := cfg.Section_Args.Remote_Flag
    remote_host := cfg.Section_Args.Etcdhost
    ssh_port := cfg.Section_Args.Ssh_Port
    remote_host_user := cfg.Section_Args.Remote_Host_User


    // Flag Handling
    if *fhost != "null"{
        etcdhost=*fhost
        remote_host=etcdhost
    }
    if *fport != "null"{
        etcdport=*fport
    }
    if *foperation != "null" {
        operation=*foperation
    }
    if *fkeycount != -1 {
        keycount = *fkeycount
    }
    if *foperation_count != -1 {
        operation_count = *foperation_count
    }
    if *flog_file != "null" {
        log_file = *flog_file
    }
    remote_flag = *fremote_flag
    mem_flag = *fmem_flag

    if remote_flag && mem_flag {
        t_key, _ := getKeyFile()
        if  err !=nil {
            panic(err)
        }
        key = t_key

        config := &ssh.ClientConfig{
            User: remote_host_user,
            Auth: []ssh.AuthMethod{
            ssh.PublicKeys(key),
            },
        }

        t_client,err := ssh.Dial("tcp", remote_host+":"+ssh_port, config)
        if err != nil {
            panic("Failed to dial: " + err.Error())
        }
        ssh_client = t_client
    }


    // Getting Memory Info for etcd instance
    if remote_flag && mem_flag {
        var bits bytes.Buffer
        mem_cmd := "pmap -x $(pidof etcd) | tail -n1 | awk '{print $4}'"
        session, err := ssh_client.NewSession()
        if err != nil {
            panic("Failed to create session: " + err.Error())
        }
        defer session.Close()
        session.Stdout = &bits
        if err := session.Run(mem_cmd); err != nil {
            panic("Failed to run: " + err.Error())
        }

        pidetcd_s = bits.String()
        pidetcd_s = strings.TrimSpace(pidetcd_s)
        etcdmem_i, _ := strconv.Atoi(pidetcd_s)
        etcdmem_s = etcdmem_i
    } else if mem_flag {
        pidtemp, _ := exec.Command("pidof","etcd").Output()
        pidetcd = string(pidtemp)
        pidetcd = strings.TrimSpace(pidetcd)
        pidetcd_s = getMemUse(pidetcd)
        etcdmem_i, _  := strconv.Atoi(pidetcd_s)
        etcdmem_s = etcdmem_i
    }
    if mem_flag {
        fmt.Println("Memory usage by etcd before requests: " + pidetcd_s +" KB")
    }
    // Creating a new client for handling requests
    var machines = []string{"http://"+etcdhost+":"+etcdport}
    client =  etcd.NewClient(machines)


    //Log File
    f, err = os.OpenFile(log_file, os.O_RDWR | os.O_CREATE , 0666)
    if err != nil {
        log.Fatalf("error opening file: %v", err)
    }
    // Log file set
    log.SetOutput(f)
    log.Println("Starting #####")
    log.Println("Keycount, operation_count =",keycount,operation_count)


    // Results : see report.go
    n := operation_count
    results = make(chan *result, n)
    

    // Keep track of the goroutines
    wg.Add(len(pct))
    
    switch{
    case operation == "create":
        log.Println("Operation : create")
        var values [2]int
        base := 0
        start := time.Now()
        for i:=0;i<len(pct);i++{
            values[0] = value_range[i]
            values[1] = value_range[i+1]
            go create_keys(base,pct_count[i],values)
        }
        wg.Wait()
        printReport(n, results, time.Now().Sub(start))
    case operation == "get":
        start := time.Now()
        log.Println("Operation : get")
        handler(get_values)
        wg.Wait()
        printReport(n, results, time.Now().Sub(start))
    case operation == "update":
        start := time.Now()
        log.Println("Operation : update")
        handler(update_values)
        wg.Wait()
        printReport(n, results, time.Now().Sub(start))
    case operation == "delete":
        start := time.Now()
        log.Println("Operation : delete")
        handler(delete_values)
        wg.Wait()
        printReport(n, results, time.Now().Sub(start))
    }
    if remote_flag && mem_flag {
        var bits bytes.Buffer
        mem_cmd := "pmap -x $(pidof etcd) | tail -n1 | awk '{print $4}'"
        session, err := ssh_client.NewSession()
        if err != nil {
            panic("Failed to create session: " + err.Error())
        }
        defer session.Close()
        session.Stdout = &bits
        if err := session.Run(mem_cmd); err != nil {
            panic("Failed to run: " + err.Error())
        }
        pidetcd_s = bits.String()
        pidetcd_s = strings.TrimSpace(pidetcd_s)
        etcdmem_i,_ := strconv.Atoi(pidetcd_s)
        etcdmem_e = etcdmem_i
    } else if mem_flag{
        pidetcd_s = getMemUse(pidetcd)
        etcdmem_i, _ := strconv.Atoi(pidetcd_s)
        etcdmem_e = etcdmem_i
    }
    if mem_flag {
        fmt.Println("Memory usage by etcd before requests: "+ pidetcd_s + " KB")
        fmt.Println("Difference := "+ strconv.Itoa(etcdmem_e - etcdmem_s)+" KB")
        log.Println("Difference in memory use, after and before load testing "+
                    ":=" + strconv.Itoa(etcdmem_e - etcdmem_s))
    }
    defer f.Close()
}

func toInt(s string, base int, bitSize int) int64 {
    i, err := strconv.ParseInt(s, base, bitSize)
    if err != nil {
        panic(err)
    }
    return i
}

func toString(i int) string {
    s := strconv.Itoa(i)
    return s
}

func get_values(base int, per_thread int){
    var key int
    limit := base + (keycount / threads)
    for i:=0;i<per_thread;i++{
        m,_ := rand.Int(rand.Reader,big.NewInt(int64(limit-base)))
        key = int(m.Int64()) + base
        start = time.Now()
        _, err := client.Get(toString(key),false,false)
        elapsed := time.Since(start)
        log.Println("key %s took %s", key, elapsed)

        //for reporting; see report.go
        var errStr string
        if err != nil {
            errStr = err.Error()
        }
        results <- &result{
            errStr:   errStr,
            duration: elapsed,
        }
    } 
    defer wg.Done()
}

func create_keys(base int, count int, r [2]int){
    var key int
    for i:=0;i<count;i++{
        key = base + i
        m,_ := rand.Int(rand.Reader,big.NewInt(int64(r[1]-r[0])))
        r1 := int(m.Int64()) + r[0]
        value := RandStringBytesRmndr(r1)
        start = time.Now()
        _, err := client.Set(toString(key),value,1000)
        elapsed := time.Since(start)
        log.Println("key %s took %s", key, elapsed)
        
        //for reporting; see report.go
        var errStr string
        if err != nil {
            errStr = err.Error()
        }
        results <- &result{
            errStr:   errStr,
            duration: elapsed,
        }
        
    }
    defer wg.Done()
}

func update_values(base int, per_thread int){
    var key int
    val := "UpdatedValue"
    limit := base + (keycount / threads)
    for i:=0;i<per_thread;i++{
        m,_ := rand.Int(rand.Reader,big.NewInt(int64(limit-base)))
        key = int(m.Int64()) + base
        start = time.Now()
        _, err := client.Set(toString(key),val,1000) 
        elapsed := time.Since(start)
        log.Println("key %s took %s", key, elapsed)

        //for reporting; see report.go
        var errStr string
        if err != nil {
            errStr = err.Error()
        }
        results <- &result{
            errStr:   errStr,
            duration: elapsed,
        }
    }
    defer wg.Done()
}

func delete_values(base int, per_thread int){
    var key int
    limit := base + (keycount / threads)
    for i:=0;i<per_thread;i++{
        m,_ := rand.Int(rand.Reader,big.NewInt(int64(limit-base)))
        key = int(m.Int64()) + base
        start = time.Now()
        _, err := client.Delete(toString(key),false)
        elapsed := time.Since(start)
        log.Println("key %s took %s", key, elapsed)

        //for reporting; see report.go
        var errStr string
        if err != nil {
            errStr = err.Error()
        }
        results <- &result{
            errStr:   errStr,
            duration: elapsed,
        }
    }
    defer wg.Done()
}


func handler(fn actions){
    per_thread := operation_count/threads
    base := 0
    for i:=0;i<threads;i++{
        go fn(base,per_thread)
        base = base + (keycount/threads)
    }
}

func RandStringBytesRmndr(n int) string {
    const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
    b := make([]byte, n)
    for i := range b {
        m,_ := rand.Int(rand.Reader,big.NewInt(int64(len(letterBytes))))
        b[i] = letterBytes[int(m.Int64())]
    }
    return string(b)
}

func getMemUse(pid string) string{
    mem, _ := exec.Command("pmap","-x",pid).Output()
    mempmap := string(mem)
    temp := strings.Split(mempmap,"\n")
    temp2 := temp[len(temp)-2]
    temp3 := strings.Fields(temp2)
    memory := temp3[3]
    return memory
}

func timeTrack(start time.Time, name string) {
    elapsed := time.Since(start)
    log.Println("key %s took %s", name, elapsed)
}

func getKeyFile() (key ssh.Signer, err error){
    //fmt.Println("getkey file funciton")
    file := os.Getenv("HOME") + "/.ssh/id_rsa"
    buf, err := ioutil.ReadFile(file)
    if err != nil {
        return
    }
    key, err = ssh.ParsePrivateKey(buf)
    if err != nil {
        return
     }
    return
}
