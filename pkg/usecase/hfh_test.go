package usecase

import (
	"testing"

	pb "github.com/scanoss/papi/api/scanningv2"

	ldb "scanoss.com/hfh-api/pkg/usecase/ldb"
)

func TestHFHscanHash(t *testing.T) {
	hfhTable, err := ldb.NewTableFromCfg("./ldb/test", "test_kb", "hfh", []string{"fileNames", "fileContents", "url"})
	if err != nil {
		t.Fatalf("an error '%s' was not expected reading the table config", err)
		return
	}
	fileNamesSimhash := "800000c9bc6bdfcd"
	result, distance, content, err := scanHash(hfhTable, fileNamesSimhash, 0, 30)
	t.Logf("Result: %x", result)
	t.Log("distance:", distance)
	t.Log("content:", content)

	if distance > 0 {
		t.Errorf("the hashes do not match: %x vs %x", fileNamesSimhash, result[0])
	}
	fileNamesSimhash = "801f1fd9bc5bdfad"

	result, distance, content, err = scanHash(hfhTable, fileNamesSimhash, 0, 30)
	t.Logf("Result: %x", result)
	t.Log("distance:", distance)
	t.Log("content:", content)
	expectedDistance := 11
	if distance != expectedDistance {
		t.Errorf("the expected distance do not match: %d vs %d", distance, expectedDistance)
	}
}

func TestHFHscanFirstStage(t *testing.T) {
	scanner, err := NewHFFHScan(24, false, "/data/ldb/", "hfh_kb")
	if err != nil {
		t.Errorf("unexpected error during initialization %v", err)
	}

	fileNamesSimhash := "8467f50e64828bb0"
	fileContentsSimhash := "8be1b3e7a672d96c"
	result, err := scanner.scanFirstStage(fileNamesSimhash, fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}

	t.Log("result:", result)

	if result.probability < 100 {
		t.Errorf("unexpected confidence result: %.1f, expected 100", result.probability)
		return
	}

	fileNamesSimhash = "8467f50e64828bb0"
	fileContentsSimhash = "8be1b2e7a672d96b"
	result, err = scanner.scanFirstStage(fileNamesSimhash, fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}

	t.Log("result:", result)

	if result.probability < 60 {
		t.Errorf("unexpected confidence result: %.1f, expected 100", result.probability)
		return
	}
}

func TestHFHscanTreeFirstStage(t *testing.T) {
	scanner, err := NewHFFHScan(70, false, "/data/ldb/", "hfh_kb")
	if err != nil {
		t.Errorf("unexpected error during initialization %v", err)
		return
	}

	node := &pb.HFHRequest_Children{
		PathId:         "/root",
		SimHashNames:   "9172bd3ef2ab37b0",
		SimHashContent: "8fce1505d6bbc965",
		Children: []*pb.HFHRequest_Children{
			{
				PathId:         "/root/child1",
				SimHashNames:   "d7fbe83ee4bdfefc",
				SimHashContent: "b2ffa1047e0c64a3",
			},
			{
				PathId:         "/root/child2",
				SimHashNames:   "8ee95581318be650",
				SimHashContent: "c4793d04a8785dec",
			},
			{
				PathId:         "/root/child3",
				SimHashNames:   "8382b543602ba3b8",
				SimHashContent: "89d3ff8dd2ad4840",
			},
		},
	}

	err = scanner.scanTreeFirstStage(node)
	if err != nil {
		t.Errorf("unexpected error during scan process %v", err)
		return
	}
	t.Log(scanner.resultsMap)
	expectedPurl := "pkg:github/mirror/busybox"
	if scanner.resultsMap["/root"].components[0].Purl != expectedPurl {
		t.Errorf("result component doesn't match: %s vs %s", scanner.resultsMap["/root"].components[0].Purl, expectedPurl)
	}
	//clean the result map
	scanner.resultsMap = make(map[string]HFHscanResult)

	node = &pb.HFHRequest_Children{
		PathId:         "/root",
		SimHashNames:   "81517c5492aa0fc8",
		SimHashContent: "f98fc3f728a8b4d4",
		Children: []*pb.HFHRequest_Children{
			{
				PathId:         "/root/child1",
				SimHashNames:   "788dae1ddd6b737f",
				SimHashContent: "3937f3d66ca89ffc",
			},
			{
				PathId:         "/root/child2",
				SimHashNames:   "841b7c7791ea27e8",
				SimHashContent: "808ec3a921ada497",
			},
			{
				PathId:         "/root/child3",
				SimHashNames:   "60f56d190b871b42",
				SimHashContent: "7c3b5bd6a67a7458",
			},
		},
	}
	err = scanner.scanTreeFirstStage(node)
	if err != nil {
		t.Errorf("unexpected error during scan process %v", err)
		return
	}
	t.Log(scanner.resultsMap)
	expectedPurl = "pkg:github/recastnavigation/recastnavigation"
	if scanner.resultsMap["/root/child3"].components[0].Purl != expectedPurl {
		t.Errorf("result component doesn't match: %s vs %s", scanner.resultsMap["/root/child3"].components[0].Purl, expectedPurl)
	}
}
