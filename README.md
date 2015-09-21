This is a etcd load test module written in go-language. It basically creates 
artificial load on a running etcd instance. The test runs for a random set of
keys and values, which can be specified in the configuration file. You can 
configure many other parameters in the config file. See the sample config file 
--"etcd_load.cfg" for more information. 

Here is what a sample config file looks like :
##################################################################
	[section-args]
	etcdhost="127.0.0.1"
	etcdport="4001"
	operation=create
	keycount="100"
	operation-count="200"
	log-file=log
	threads=5 
	pct="5,74,10,10,1"
	value-range="0,256,512,1024,8192,204800"
	remote-flag=False
	ssh-port=22
	remote-host-user=root
##################################################################


Features include :
 - Memory information
  - gives the memory information about the running etcd instance -- before, 
  	after and difference, when requests are made
  -	To get this info, use the "-mem" flag while running the module
  - Also, if the instance is running on a remote machine, then you need to use
  	the "-remote" flag as well.
 - Key value distribution
  - This feature basically allows you to specify the distribution of the 
  	key-values, that is how many keys lie in a particular value range, specified
  	by "value-range" parameter under section : "section-args", in etcd_load.cfg
  - See the "pct" parameter under the section :"section-args", in etcd_load.cfg
 - Value range
  - This allows you to specify value ranges, which will be used for the -- Key
  	value distribution feature.
  - Note : length (Value Range) = length (Key Value distribution) + 1
 - There are other configuration options as well, like -- log-file, remote-flag,
 	remote-host-user, etcd. . Some of these can be specified using commandline 
 	flags as well, in which case the flags will override the cfg-file values. To
 	know more about them see the default config file -- "etcd_load.cfg" and for
 	help regarding flags, use
 		- go run etcd_load.go -h

Before you proceed there are a few things that you might need to set up.

 - Make sure that the following packages are in your GOPATH. Set your 
   GOPATH if its not already set.
   In go, to get a "package" you can simply do : go get package. 
   In this use case do it under your GOPATH.

	Packages :
		"github.com/coreos/go-etcd/etcd"
		"code.google.com/p/gcfg"
		"code.google.com/p/go.crypto/ssh"


 - Set up a default config file, like the one available in the repo.
 	The details on how to configure it are in the sample -- "etcd_laod.cfg" 
 	itself

Now, to run etcd_load test, use the following steps
 - go build etcd_load.go report.go
 - ./etcd_load -c "default-config-file" --other-optional-flags
 - Examples :
 	- ./etcd_load -c etcd_load.go -mem -remote -o create  ----------# [remote etcd instance]
 	- ./etcd_load -c etcd_load.go -h 10.10.10.1 -p 4001 -o create ---------# [remote etcd instance]
 	- ./etcd_load -c etcd_load.go -h 127.0.0.1 -o create -----------# [local etcd instance]

	Note that the "-c" flag is compulsory, that is you need to have a default 
	config file that must be input using the -c flag
	To know more about the flags :: do -- go run etcd_load.go -h

Credit ::
 - The report.go file in the package is used from the "boom" package, here is the link
 	- https://github.com/rakyll/boom/blob/master/boomer/print.go

 
You can find more runtime details in the log file. 
The commandline the report looks like :
**********************************************************************
	Summary:
	  Total:	2.8454 secs.
	  Slowest:	0.1493 secs.
	  Fastest:	0.0001 secs.
	  Average:	0.0258 secs.
	  Requests/sec:	35.1449

	Response time histogram:
	  0.000 [1]		|
	  0.015 [17]	|∎∎∎∎∎∎∎∎∎∎∎∎∎∎
	  0.030 [48]	|∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎
	  0.045 [27]	|∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎
	  0.060 [3]		|∎∎
	  0.075 [2]		|∎
	  0.090 [0]		|
	  0.105 [1]		|
	  0.119 [0]		|
	  0.134 [0]		|
	  0.149 [1]		|

	Latency distribution:
	  10% in 0.0006 secs.
	  25% in 0.0226 secs.
	  50% in 0.0240 secs.
	  75% in 0.0308 secs.
	  90% in 0.0332 secs.
	  95% in 0.0481 secs.
	  99% in 0.1493 secs.

**********************************************************************
