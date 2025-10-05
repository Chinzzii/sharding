# **Go Database Sharding Demo**

This project provides a hands-on demonstration of the performance impact of database sharding. Using Go and SQLite, it simulates a high-concurrency environment to compare the throughput and latency of a single, monolithic database versus a horizontally sharded database architecture under various workloads.

The goal is to provide clear, quantitative data that illustrates not only the benefits of sharding but also its real-world trade-offs.

## **Key Concepts**

**Database Sharding** is a database architecture pattern where a large database is horizontally partitioned into smaller, faster, more manageable pieces called shards. Each shard is a separate database, and the application logic distributes data among them. This is a common technique for scaling applications that need to handle a high volume of reads and writes.

## **Project Structure**

* /cmd/demo-server: A simple HTTP server written in Go that exposes /insert and /get endpoints. It can run in either single or sharded mode.  
* /cmd/loadgen: A load generation client that simulates concurrent users, sending a high volume of requests to the demo server to measure performance.  
* /internal/sharding: The core sharding logic. It uses a CRC32 hash of the User ID to determine which shard a given piece of data belongs to.  
* /scripts: Bash scripts to automate the entire testing process, from building the binaries to running scenarios and collecting results.  
* /results: The output directory where performance data is saved in CSV format.

## **How to Run the Experiments**

This project is designed to be run from the command line.

### **Prerequisites**

* Go (version 1.18 or later)  
* A Unix-like environment (for the bash scripts)

### **Step 1: Build the Binaries**

The automation script handles this, but you can also build them manually:

go build \-o ./bin/demo-server ./cmd/demo-server  
go build \-o ./bin/loadgen ./cmd/loadgen

### **Step 2: Run All Scenarios**

The easiest way to get started is to run the entire test suite. This will execute a series of pre-configured tests, running each one against both a single database and a 3-shard configuration. It will also run a special scaling test with 5 shards.

Make the script executable, then run it:

chmod \+x ./scripts/run\_all\_scenarios.sh  
./scripts/run\_all\_scenarios.sh

Results will be appended to ./results/results.csv.

### **Step 3: Run a Custom Scenario**

You can also run a single experiment with custom parameters using run\_scenario.sh.

chmod \+x ./scripts/run\_scenario.sh

\# Example: Run a test with 50,000 operations and 150 concurrent users  
./scripts/run\_scenario.sh \--ops 50000 \--concurrency 150 \--writeRatio 0.5 \--shards 4

## **Key Findings & Analysis**

The experiments clearly demonstrate the power and pitfalls of database sharding. Below is a summary of the most insightful results from the test run.

| Scenario Name | Mode | Shards | Throughput (ops/s) | Avg. Latency (ms) | Notes |
| :---- | :---- | :---- | :---- | :---- | :---- |
| **read\_heavy\_cachelike** | single | 1 | 7,983 | 7.00 | **Baseline for read performance** |
|  | sharded | 3 | **15,538** | **4.34** | **\~95% throughput increase** |
| **sustained\_stress** | single | 1 | 5,348 | 35.57 | **Baseline for high load** |
|  | sharded | 3 | **9,608** | **19.68** | **\~80% throughput increase** |
| **write\_heavy\_ingest** | single | 1 | 4,519 | 8.27 | **Baseline for write performance** |
|  | sharded | 3 | **4,901** | 9.31 | **Modest gain, higher latency** |
| **shard\_scaling\_base** | single | 1 | 5,857 | 14.53 | **Baseline for scaling test** |
|  | sharded | 3 | **7,030** | **13.00** | **Good performance with 3 shards** |
|  | sharded | **5** | 4,786 | 18.80 | **Performance *decreased* with 5 shards** |

### **Analysis**

1. **Sharding Excels for Read-Heavy & High-Concurrency Loads**: The read\_heavy\_cachelike and sustained\_stress scenarios show massive performance gains. By distributing reads across multiple independent databases, the system can parallelize I/O operations and avoid lock contention, nearly doubling the throughput. This is the classic use case and primary benefit of sharding.  
2. **Bottlenecks Simply Move**: In the write\_heavy\_ingest scenario, the performance gain was minimal, and latency actually increased slightly. This demonstrates that sharding is not a magic bullet. The bottleneck became the write performance of *each individual shard*. When every shard is overwhelmed with writes, the overall system still struggles.  
3. **More Shards Are Not Always Better**: The shard\_scaling\_base test revealed the most critical insight: increasing shards from 3 to 5 *hurt* performance, making it even worse than the single database. This is due to **sharding overhead**. Each additional shard adds costs in connection management, CPU context switching, and file handle limits. This experiment proves there is a "sweet spot" for the number of shards, and over-sharding can be counterproductive.

## **Conclusion**

This project successfully demonstrates that database sharding is a powerful technique for scaling applications but requires careful consideration. It provides the most significant benefits for read-heavy and mixed-workload systems by distributing contention. However, engineers must be mindful of the trade-offs, including diminishing returns for write-heavy loads and the performance cost of excessive sharding.