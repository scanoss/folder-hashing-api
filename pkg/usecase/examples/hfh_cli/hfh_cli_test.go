package hfh_cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pb "github.com/scanoss/papi/api/scanningv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func startServer(t *testing.T) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	cmd := exec.Command("go", "run", "../../../../cmd/server/main.go",
		"-json-config", "../../../../config/app-config-dev.json", "-debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Start()
	if err != nil {
		return nil, nil, nil, err
	}
	time.Sleep(5 * time.Second)
	return cmd, &stdout, &stderr, nil
}
func killProcessOnPort(port string) error {
	// En Linux/Unix podemos usar lsof
	cmd := exec.Command("lsof", "-t", "-i", ":"+port)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pid := strings.TrimSpace(string(output))
		killCmd := exec.Command("kill", "-9", pid)
		return killCmd.Run()
	}
	return nil
}

func waitForPortToBeFreed(t *testing.T, port string) error {
	t.Log("Waiting for port to be freed...")

	// Primero intentamos matar cualquier proceso que esté usando el puerto
	if err := killProcessOnPort(port); err != nil {
		t.Logf("Warning: Failed to kill process on port %s: %v", port, err)
	}

	// Ahora esperamos a que el puerto esté realmente libre
	timeout := time.After(10 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("port %s did not free up in time", port)
		case <-tick.C:
			conn, err := net.Listen("tcp", ":"+port)
			if err == nil {
				conn.Close()
				t.Log("Port is now free")
				return nil
			}
			t.Logf("Port still in use, retrying... (error: %v)", err)
		}
	}
}
func TestLocalGRPCRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := waitForPortToBeFreed(t, "50061"); err != nil {
		t.Fatalf("Port is not free at start: %v", err)
	}

	requestRoot, err := HFHrequestFromPath("../")
	if err != nil {
		t.Error(err)
		return
	}

	request := &pb.HFHRequest{
		BestMatch: false,
		Threshold: 100,
		Root:      requestRoot,
	}

	t.Log("Starting server...")
	serverCmd, stdout, stderr, err := startServer(t)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	var wg sync.WaitGroup
	done := make(chan bool)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			default:
				if stdout.Len() > 0 {
					t.Logf("Server stdout: %s", stdout.String())
					stdout.Reset()
				}
				if stderr.Len() > 0 {
					t.Logf("Server stderr: %s", stderr.String())
					stderr.Reset()
				}
				time.Sleep(time.Second)
			}
		}
	}()

	// Declarar conn en el scope correcto
	var conn *grpc.ClientConn

	// Mejorar la limpieza
	defer func() {
		t.Log("Starting cleanup...")

		// 1. Cerrar la conexión gRPC primero si existe
		if conn != nil {
			t.Log("Closing gRPC connection...")
			conn.Close()
		}

		// 2. Detener la goroutine de monitoreo
		close(done)
		t.Log("Waiting for monitor goroutine to finish...")
		wg.Wait()

		// 3. Enviar SIGTERM primero para un apagado graceful
		if serverCmd.Process != nil {
			t.Log("Sending SIGTERM to server...")
			if err := serverCmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Logf("Failed to send SIGTERM: %v", err)
			}

			// Esperar un poco para que el servidor se apague gracefully
			termCtx, termCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer termCancel()

			done := make(chan error)
			go func() {
				done <- serverCmd.Wait()
			}()

			select {
			case <-termCtx.Done():
				t.Log("Server didn't shut down gracefully, using SIGKILL...")
				if err := serverCmd.Process.Kill(); err != nil {
					t.Logf("Failed to kill server process: %v", err)
					if pgid, err := syscall.Getpgid(serverCmd.Process.Pid); err == nil {
						syscall.Kill(-pgid, syscall.SIGKILL)
					}
				}
			case err := <-done:
				if err != nil {
					t.Logf("Server process ended with error: %v", err)
				} else {
					t.Log("Server process ended successfully")
				}
			}
		}

		// 4. Esperar a que el puerto se libere
		if err := waitForPortToBeFreed(t, "50061"); err != nil {
			t.Logf("Warning: %v", err)
		}

		t.Log("Cleanup completed")
	}()

	// Establecer la conexión gRPC
	t.Log("Establishing gRPC connection...")
	conn, err = grpc.Dial("localhost:50061",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("did not connect: %v", err)
		return
	}

	t.Log("Creating client and sending request...")
	client := pb.NewScanningClient(conn)
	response, err := client.FolderHashScan(ctx, request)
	if err != nil {
		t.Fatalf("could not scan: %v", err)
		return
	}

	t.Log("Processing response...")
	t.Logf("Response Status: %+v", response.Status)
	jsonBytes, _ := json.Marshal(response)
	t.Log(string(jsonBytes))
}
