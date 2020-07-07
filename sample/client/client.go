package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	// The Protobuf generated file
	creator "github.com/xin-hedera/haproxy-grpc-sample/sample/codenamecreator"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var (
	server     string
	tps        int
	numWorkers int
	size       int
	duration   time.Duration

	wg sync.WaitGroup
)

func main() {

	mainCmd := &cobra.Command{
		Short: "Run grpc client",
		RunE:  benchmark,
	}

	// flags
	mainCmd.PersistentFlags().StringVarP(&server, "server", "s", "", "server address, IP:port")
	mainCmd.PersistentFlags().IntVarP(&tps, "tps", "t", 10, "target TPS (transactions / second)")
	mainCmd.PersistentFlags().IntVarP(&numWorkers, "workers", "w", 2, "number of workers")
	mainCmd.PersistentFlags().IntVar(&size, "size", 100, "payload size in bytes")
	mainCmd.PersistentFlags().DurationVarP(&duration, "duration", "d", time.Minute, "duration to run the benchmark")

	mainCmd.Execute()
}

func getSingleCodenameAndExitExample(ctx context.Context, client creator.CodenameCreatorClient, category string, message []byte) bool {
	_, err := client.GetCodename(ctx, &creator.NameRequest{Category: category, Unused: message})
	if err != nil {
		log.Errorf("Could not get result, %s", err)
		return false
	}
	//log.Printf("Codename result: %s", result)
	return true
}

func getCodenamesStreamingExample(ctx context.Context, client creator.CodenameCreatorClient) {
	fmt.Println("Generating codenames...")
	stream, err := client.KeepGettingCodenames(ctx)

	if err != nil {
		log.Fatalf("Could not get stream, %v", err)
	}

	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}

			if err != nil {
				log.Fatalf("%v", err)
			}

			log.Printf("Received: %s\n", in.Name)
		}
	}()

	category := "Science"
	for {
		if category == "Science" {
			category = "Animals"
		} else {
			category = "Science"
		}

		err := stream.Send(&creator.NameRequest{Category: category})
		if err != nil {
			log.Fatalf("%v", err)
		}
		time.Sleep(10 * time.Second)
	}
}

func benchmark(cmd *cobra.Command, args []string) error {
	initLogger()

	if server == "" {
		return fmt.Errorf("must provide server address")
	}

	if numWorkers < 1 || numWorkers > 200 {
		return fmt.Errorf("invalid number of workers, %d", numWorkers)
	}

	if size < 1 || size > 10240 {
		return fmt.Errorf("bad size - %d", size)
	}

	//crt := os.Getenv("TLS_CERT")
	//
	//creds, err := credentials.NewClientTLSFromFile(crt, "")
	//if err != nil {
	//	log.Fatalf("Failed to load TLS certificate")
	//}

	conn, err := grpc.Dial(server, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("dd not connect, %s", err)
	}
	defer conn.Close()

	client := creator.NewCodenameCreatorClient(conn)
	//ctx := context.Background()

	wg.Add(numWorkers)
	start := time.Now()
	counts := make([]int, numWorkers)
	tpsPerWorker := float64(tps) / float64(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(index int) {
			counts[index] = worker(client, tpsPerWorker, duration, size, index)
		}(i)
	}
	wg.Wait()
	elapsed := time.Now().Sub(start)

	total := 0
	for _, count := range counts {
		total += count
	}
	fmt.Printf("%d requests, tps - %.2f\n", total, float64(total)/elapsed.Seconds())
	// Optional: add some metadata
	//ctx = metadata.AppendToOutgoingContext(ctx, "mysecretpassphrase", "abc123")

	//getCodenamesStreamingExample(ctx, client)
	//getSingleCodenameAndExitExample(ctx, client, "Science")
	return nil
}

func worker(client creator.CodenameCreatorClient, tps float64, duration time.Duration, size int, index int) int {
	log.Infof("worker %d just started...", index)
	until := time.After(duration)
	start := time.Now()
	message := make([]byte, size)
	success := 0
	failure := 0

	var ticker *time.Ticker
	if duration < 100*time.Second {
		ticker = time.NewTicker(time.Second)
	} else {
		ticker = time.NewTicker(10 * time.Second)
	}

	for {
		done := false
		select {
		case <-until:
			done = true
		case now := <-ticker.C:
			elapsed := now.Sub(start)
			log.Infof("worker(%4d): %.2f seconds, %d sent, tps - %.2f", index, elapsed.Seconds(), success, float64(success)/elapsed.Seconds())
			//continue
		//case <-sendTimer.C:
		default:
		}

		if done {
			ticker.Stop()
			break
		}

		if getSingleCodenameAndExitExample(context.TODO(), client, "Science", message) {
			success++
		} else {
			failure++
		}

		elapsed := time.Now().Sub(start)
		curTps := float64(success) / elapsed.Seconds()
		if curTps >= tps {
			overflow := float64(success) - tps*elapsed.Seconds()
			time.Sleep(time.Duration(overflow/curTps) * time.Second)
		}
	}

	wg.Done()
	log.Infof("worker(%4d): success - %d, failure - %d", index, success, failure)

	return success
}

func initLogger() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})
}
