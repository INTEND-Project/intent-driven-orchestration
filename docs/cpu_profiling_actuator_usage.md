# CPU Profiling Actuator Usage

CPU profiling actuator enables allocating CPU resources to workloads based on qualitative aspects that can be either related to hardware (e.g., different CPU SKUs, generations, etc.), allocation strategies (e.g., shared vs pinned CPUs) or other CPU configuation aspects.

## 1. Concept 
* The nodes in the cluster have different CPU specifications, whether due to hardware variations or configuration differences, called ***CPU profiles***.

* Each node has one CPU profile. Multiple nodes can have the same CPU profile.

* At the Kubernetes level, the nodes are labeled based on their corresponding CPU profile. 

* The current implementation uses the Kubernetes CPU manager, where the actuator applies node affinity rules to select the targeted CPU profile. 

## 2. Configuration   
This actuator uses standard actuator settings plus an additional cpu_profiles field, which specifies an array of all CPU profiles available in the cluster. Each CPU profile is defined according to the parameters shown in the table below.

| Key | Value | Description |
|-----|------|-------------|
| id | \<int> | ids start from 1 and must be sequential (no gaps). CPU profiles with lower ids are more cost-effective.
|name | \<string> | (optional) Used for profile description purposes only. 
cpu_manager|0|CPU manager identifier. Currently only supports vanilla Kubernetes CPU manager (value: 0).|
|affinity | 0 \| 1  | Node affinity mode: 1 (required) for optimal ML accuracy;  0 (preferred) to avoid resource blocking.
|settings |{\<string>: \<string>} |Key-value pairs representing CPU specifications matching Kubernetes node labels. Each key-value pair must be unique across all CPU profiles to avoid conflicts. |  

