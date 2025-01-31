package hfh_cli

import (
	"context"
	"encoding/json"
	"log"
	"testing"
	"time"

	pb "github.com/scanoss/papi/api/scanningv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestHFHcliBuildRequest(t *testing.T) {
	request := HFHrequestFromPath("./")
	t.Log(request)
}

func TestLocalGRPCRequest(t *testing.T) {

	conn, err := grpc.Dial("localhost:50061", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	// Crear el cliente
	client := pb.NewScanningClient(conn)

	// Crear el contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1000)
	defer cancel()
	requestRoot := HFHrequestFromPath("/data/mariano/test_projects/rapidjson-1.0.2")
	request := &pb.HFHRequest{
		BestMatch: true,
		Threshold: 60,
		Root:      requestRoot,
	}
	// Hacer la llamada
	response, err := client.FolderHashScan(ctx, request)
	if err != nil {
		log.Fatalf("could not scan: %v", err)
	}

	// Procesar la respuesta
	t.Logf("Response Status: %+v", response.Status)
	jsonBytes, _ := json.Marshal(response)
	t.Log(string(jsonBytes))

}
