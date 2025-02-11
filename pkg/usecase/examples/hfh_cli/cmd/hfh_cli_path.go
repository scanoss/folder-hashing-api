package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	pb "github.com/scanoss/papi/api/scanningv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	cli "scanoss.com/hfh-api/pkg/usecase/examples/hfh_cli"
)

func directoryExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
func main() {
	detailed := flag.Bool("detailed", false, "request a detailed scan")
	flag.Parse()
	path := flag.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Minute)
	defer cancel()

	if !directoryExists(path) {
		println("The specified path is not valid")
		os.Exit(1)
	}

	requestRoot, err := cli.HFHrequestFromPath(path)
	fmt.Println(err)

	request := &pb.HFHRequest{
		BestMatch: *detailed,
		Threshold: 100,
		Root:      requestRoot,
	}

	var conn *grpc.ClientConn

	log.Printf("Establishing gRPC connection...")
	conn, err = grpc.Dial("localhost:50061",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("did not connect: %v", err)
		return
	}

	log.Printf("Creating client and sending request...")
	client := pb.NewScanningClient(conn)
	response, err := client.FolderHashScan(ctx, request)
	if err != nil {
		log.Fatalf("could not scan: %v", err)
		return
	}

	log.Printf("Processing response...")
	log.Printf("Response Status: %+v", response.Status)
	jsonBytes, _ := json.MarshalIndent(response, "", "\t")
	fmt.Println(string(jsonBytes))

}
