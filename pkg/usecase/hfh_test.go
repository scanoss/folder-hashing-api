package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"

	zlog "github.com/scanoss/zap-logging-helper/pkg/logger"
	myconfig "scanoss.com/hfh-api/pkg/config"
	"scanoss.com/hfh-api/pkg/dtos"
	ldb "scanoss.com/hfh-api/pkg/usecase/ldb"
	test "scanoss.com/hfh-api/pkg/usecase/test"
)

func testScanInitHelper() (*HFHscan, error) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		return nil, fmt.Errorf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := ctxzap.ToContext(context.Background(), zlog.L)
	s := ctxzap.Extract(ctx).Sugar()
	//add feeader for ldb test
	/*var feeders []config.Feeder

	feeders = append(feeders, feeder.Json{Path: "./test/test_config.json"})

	//load default config
	cfg, err := myconfig.NewServerConfig(feeders)
	if err != nil {
		return nil, fmt.Errorf("Fatal error loading default config")
	}*/

	cfg, err := myconfig.NewServerConfig(nil)
	if err != nil {
		return nil, fmt.Errorf("Fatal error loading default config")
	}

	scanner, err := HFHScanInit(s, cfg)
	if err != nil {
		return nil, fmt.Errorf("error during scanner initialization")
	}
	return scanner, nil
}

func TestHFHscanHash(t *testing.T) {
	hfhTable, err := ldb.NewTableFromCfg("", "hfh_kb", "hfh", []string{"fileNames", "fileContents", "url"})
	if err != nil {
		t.Fatalf("an error '%s' was not expected reading the table config", err)
		return
	}
	fileNamesSimhash := "8172bd3ef0ab37b4"
	result, distance, content, err := scanHash(hfhTable, fileNamesSimhash, 0, 30)
	t.Logf("Result: %x", result)
	t.Log("distance:", distance)
	t.Log("content:", content)

	if distance > 0 {
		t.Errorf("the hashes do not match: %x vs %x", fileNamesSimhash, result[0])
	}
	fileNamesSimhash = "8162bd4ec1aa36b3"

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

	fileNamesSimhash := "8172bd3ef0ab37b4"
	fileContentsSimhash := "f98fc3f728a8b4d4"
	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
	}
	result, err := scanner.scanFirstStage(fileNamesSimhash, fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}

	t.Log("result:", result.Components)
	var expectedProb float32 = 100.0

	if result.Probability < expectedProb {
		t.Errorf("unexpected confidence result: %.1f, expected %.1f", result.Probability, expectedProb)
	}
	expectedPurl := "pkg:github/mirror/busybox"
	if result.Components[0].Purl != expectedPurl {
		t.Errorf("unexpected purl result: %s, expected %s", result.Components[0].Purl, expectedPurl)
	}

	fileNamesSimhash = "8172bd3ef1ab37b1"
	fileContentsSimhash = "f98fc3f728a8b4d5"
	result, err = scanner.scanFirstStage(fileNamesSimhash, fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}

	t.Log("result:", result)
	expectedProb = 87.5
	if result.Probability < expectedProb {
		t.Errorf("unexpected confidence result: %.1f, expected %.1f", result.Probability, expectedProb)
	}
}

func TestHFHscanTreeFirstStage(t *testing.T) {
	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanner.resultsMap = make(map[string]HFHscanResult)

	node := &dtos.HFHScanInputChildren{
		PathId:         "/root",
		SimHashNames:   "8172bd3ef0ab37b4",
		SimHashContent: "862e8c6490b29efd",
		Children: []*dtos.HFHScanInputChildren{
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
	if scanner.resultsMap["/root"].Components[0].Purl != expectedPurl {
		t.Errorf("result component doesn't match: %s vs %s", scanner.resultsMap["/root"].Components[0].Purl, expectedPurl)
	}
	//clean the result map
	scanner.resultsMap = make(map[string]HFHscanResult)

	node = &dtos.HFHScanInputChildren{
		PathId:         "/root",
		SimHashNames:   "4f3dcd05ab272b52",
		SimHashContent: "543a57d3b6f27064",
		Children: []*dtos.HFHScanInputChildren{
			{
				PathId:         "/root/child1",
				SimHashNames:   "9be5af29fa0f73eb",
				SimHashContent: "4fabf920800f6dc",
			},
			{
				PathId:         "/root/child2",
				SimHashNames:   "775c095780b58a42",
				SimHashContent: "645ad6b1beb7f129",
			},
			{
				PathId:         "/root/child3",
				SimHashNames:   "5f3dcd05ab272b52",
				SimHashContent: "643a57d3b6f27064",
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
	if scanner.resultsMap["/root/child3"].Components[0].Purl != expectedPurl {
		t.Errorf("result component doesn't match: %s vs %s", scanner.resultsMap["/root/child3"].Components[0].Purl, expectedPurl)
	}
}

func TestHFHscanSecondStage(t *testing.T) {
	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	var expectedProb float32 = 100.0
	fileContentsSimhash := "644789ffa01b315e"
	result, err := scanner.scanSecondStage(fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}

	t.Log("result:", result.Components)

	if result.Probability < expectedProb {
		t.Errorf("unexpected confidence result: %.1f, expected %.1f", result.Probability, expectedProb)
	}

	fileContentsSimhash = "76bdd0767d764853"
	result, err = scanner.scanSecondStage(fileContentsSimhash)
	if err != nil {
		t.Errorf("scan failed with error: %v", err)
	}
	expectedProb = 66.7
	t.Logf("prob %.1f - result: %v", result.Probability, result.Components)
	if result.Probability < expectedProb-1 || result.Probability > expectedProb+1 {
		t.Errorf("unexpected confidence result: %.1f, expected %.1f", result.Probability, expectedProb)
		return
	}
	expectedPurl := "pkg:github/tencent/rapidjson"
	if result.Components[0].Purl != expectedPurl {
		t.Errorf("unexpected result purl: %s, expected: %s", result.Components[0].Purl, expectedPurl)
	}
}

func TestHFHscanTreeSecondStage(t *testing.T) {
	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanner.resultsMap = make(map[string]HFHscanResult)

	node := &dtos.HFHScanInputChildren{
		PathId:         "/root",
		SimHashNames:   "8172bd3ef0ab37b4",
		SimHashContent: "862e8c6490b29efd",
		Children: []*dtos.HFHScanInputChildren{
			{
				PathId:         "/root/child1",
				SimHashNames:   "d7fbe83ee4bdfefc",
				SimHashContent: "76bdd0767d764853",
			},
			{
				PathId:         "/root/child2",
				SimHashNames:   "8ee95581318be650",
				SimHashContent: "644789ffa01b315e",
			},
			{
				PathId:         "/root/child3",
				SimHashNames:   "8382b543602ba3b8",
				SimHashContent: "643a57d3b6f27064",
			},
		},
	}

	err = scanner.scanTreeSecondStage(node)
	if err != nil {
		t.Errorf("unexpected error during scan process %v", err)
	}

	t.Log("Second stage results:", scanner.resultsMap)
	expectedPurl := "pkg:github/tencent/rapidjson"
	if scanner.resultsMap["/root/child2"].Components[0].Purl != expectedPurl {
		t.Errorf("unexpected result purl %s. Expected: %s", scanner.resultsMap["/root/child2"].Components[0].Purl, expectedPurl)
	}
}

func TestHFHthirdStep(t *testing.T) {
	resultsMap := map[string]HFHscanResult{
		"/monorepo/deps/libsignal-protocol-test/gradle": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/signalapp/libsignal-protocol-java",
					Versions:   []string{"v2.3.0"},
					Confidence: 100,
					Date:       time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 100,
		},
		"/monorepo/deps/libsignal-protocol-test/java/src": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/signalapp/libsignal-protocol-java",
					Versions:   []string{"v2.3.0"},
					Confidence: 100,
					Date:       time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 95.83333,
		},
		"/monorepo/deps/libsignal-protocol-test/tests/src": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/signalapp/libsignal-protocol-java",
					Versions:   []string{"v2.3.0"},
					Confidence: 100,
					Date:       time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 100,
		},
		"/monorepo/deps/other": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/recastnavigation/recastnavigation",
					Versions:   []string{"v1.6.0"},
					Confidence: 100,
					Date:       time.Date(2023, 5, 21, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       1,
			Probability: 100,
		},
		"/monorepo/deps/zlib-1.2.13": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:gitlab/rluna-database/nosql/arangodb/arangodb-2020",
					Versions:   []string{"v3.11.3.1"},
					Confidence: 100,
					Date:       time.Date(2023, 9, 27, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 87.5,
		},
		"/monorepo/other/rapidjson-1.1.0-test": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/nilsbore/roswasm_suite",
					Versions:   []string{"release-8"},
					Confidence: 100,
					Date:       time.Date(2021, 3, 11, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 75,
		},
		"/monorepo/other/rapidjson-1.1.0-test/bin": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/rhysd/tinyjson",
					Versions:   []string{"v1.0.0"},
					Confidence: 100,
					Date:       time.Date(2020, 4, 28, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       0,
			Probability: 41.666664,
		},
		"/monorepo/other/rapidjson-1.1.0-test/docker": {
			Components: []HFHscanResultItem{
				{
					Purl:       "pkg:github/jmenga/docker-ansible",
					Versions:   []string{"module-3-after"},
					Confidence: 100,
					Date:       time.Date(2016, 1, 29, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:github/podbox/docker-teamcity",
					Versions:   []string{"2e44b32"},
					Confidence: 100,
					Date:       time.Date(2016, 5, 6, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:bitbucket/suprocktech/docker_cortex_m",
					Versions:   []string{"1.0.0"},
					Confidence: 100,
					Date:       time.Date(2017, 9, 12, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:gitlab/gableroux/gitlab-ci-example-docker",
					Versions:   []string{"d57a02c7"},
					Confidence: 100,
					Date:       time.Date(2018, 1, 17, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:bitbucket/wprowebdev/ubuntu-node-gulp",
					Versions:   []string{"v1.0.1"},
					Confidence: 100,
					Date:       time.Date(2018, 8, 17, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:bitbucket/sarkisv/docker-git-test",
					Versions:   []string{"v4.1.0"},
					Confidence: 100,
					Date:       time.Date(2019, 6, 23, 0, 0, 0, 0, time.UTC),
				},
				// Continued with remaining docker components...
				{
					Purl:       "pkg:github/morphy2k/docker-hello-world",
					Versions:   []string{"v0.1.0-beta.5"},
					Confidence: 100,
					Date:       time.Date(2024, 10, 10, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:       "pkg:github/mohrm/traktorgaehn",
					Versions:   []string{"v0.2.0-beta4"},
					Confidence: 100,
					Date:       time.Date(2024, 11, 27, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       0,
			Probability: 50,
		},
	}

	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanInput := dtos.HFHscanInput{Threshold: 100, BestMatch: false, Root: test.Monorepo_root}
	scanner.resultsMap = resultsMap
	err = scanner.scanTreeThirdStage(scanInput.Root)
	if err != nil {
		t.Errorf("unexpected error during scan process %v", err)
		return
	}

	jsonBytes, _ := json.Marshal(scanner.resultsMap)
	scanner.s.Debug(string(jsonBytes))

	t.Log("Second stage results:", scanner.resultsMap)
	expectedPurl := "pkg:github/signalapp/libsignal-protocol-java"
	var expectedProb float32 = 59.17
	if resultsMap["/monorepo/deps/libsignal-protocol-test"].Components[0].Purl != expectedPurl {
		t.Errorf("unexpected result purl %s. Expected: %s", resultsMap["/monorepo/deps/libsignal-protocol-test"].Components[0].Purl, expectedPurl)
	}

	if resultsMap["/monorepo/deps/libsignal-protocol-test"].Probability < expectedProb-1 || resultsMap["/monorepo/deps/libsignal-protocol-test"].Probability > expectedProb+1 {
		t.Errorf("unexpected confidence result: %.1f, expected %.1f", resultsMap["/monorepo/deps/libsignal-protocol-test"].Probability, expectedProb)
	}

}

func TestHFHproduceResponse(t *testing.T) {

	resultsMap := map[string]HFHscanResult{
		"/monorepo/deps/libsignal-protocol-test/java": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/signalapp/libsignal-protocol-java",
					Versions: []string{"v2.3.0"},
				},
			},
			Stage:       1,
			Probability: 83.33333,
		},
		"/monorepo/deps/other": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/recastnavigation/recastnavigation",
					Versions: []string{"v1.6.0"},
				},
			},
			Stage:       1,
			Probability: 100,
		},
		"/monorepo/other/CSerial-0.3_test": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/rm5248/cserial",
					Versions: []string{"v0.3", "7b5cbd5"},
				},
			},
			Stage:       0,
			Probability: 50,
		},
		"/monorepo": {
			Components:  []HFHscanResultItem{},
			Stage:       2,
			Probability: 66.666664,
		},
	}

	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanner.resultsMap = resultsMap
	node := test.Monorepo_root
	var results dtos.HFHResultOutput
	scanner.produceResults(node, &results.Results)
	jsonBytes, _ := json.Marshal(scanner.resultsMap)
	scanner.s.Debug(string(jsonBytes))
	expectedResponse := `{"/monorepo":{"components":[],"Stage":2,"probability":66.666664},"/monorepo/deps/libsignal-protocol-test/java":{"components":[{"purl":"pkg:github/signalapp/libsignal-protocol-java","versions":["v2.3.0"],"confidence":0,"date":"0001-01-01T00:00:00Z"}],"Stage":1,"probability":83.33333},"/monorepo/deps/other":{"components":[{"purl":"pkg:github/recastnavigation/recastnavigation","versions":["v1.6.0"],"confidence":0,"date":"0001-01-01T00:00:00Z"}],"Stage":1,"probability":100},"/monorepo/other/CSerial-0.3_test":{"components":[{"purl":"pkg:github/rm5248/cserial","versions":["v0.3","7b5cbd5"],"confidence":0,"date":"0001-01-01T00:00:00Z"}],"Stage":0,"probability":50}}`
	if string(jsonBytes) != expectedResponse {
		t.Errorf("unexpected response %s. Expected: %s", string(jsonBytes), expectedResponse)
	}
}
func TestHFHScan(t *testing.T) {
	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanInput := dtos.HFHscanInput{Threshold: 100.0, BestMatch: false, Root: test.Monorepo_root}
	response, err := scanner.Scan(&scanInput)
	if err != nil {
		t.Errorf("scannning fails %v", err)
		return
	}
	jsonBytes, _ := json.Marshal(response)
	t.Log(string(jsonBytes))
	expectedResponse := `{"results":[{"path_id":"/monorepo/deps/libsignal-protocol-test","components":[{"purl":"pkg:github/signalapp/libsignal-protocol-java","versions":["pkg:github/signalapp/libsignal-protocol-java"],"confidence":59.166664}]},{"path_id":"/monorepo/deps/other","components":[{"purl":"pkg:github/recastnavigation/recastnavigation","versions":["v1.6.0"],"confidence":100}]},{"path_id":"/monorepo/deps/zlib-1.2.13","components":[{"purl":"pkg:gitlab/rluna-database/nosql/arangodb/arangodb-2020","versions":["v3.11.3.1"],"confidence":100}]},{"path_id":"/monorepo/other/rapidjson-1.1.0-test","components":[{"purl":"pkg:github/nilsbore/roswasm_suite","versions":["release-8"],"confidence":100}]}]}`
	if string(jsonBytes) != expectedResponse {
		t.Errorf("unexpected response %s. Expected: %s", string(jsonBytes), expectedResponse)
	}
}

func TestHFHScanBusyBox(t *testing.T) {

	node := &dtos.HFHScanInputChildren{
		PathId:         "/root",
		SimHashNames:   "8172bd3ef0ab37b4",
		SimHashContent: "862e8c6490b29efd",
		Children: []*dtos.HFHScanInputChildren{
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

	scanner, err := testScanInitHelper()
	if err != nil {
		t.Fatal(err)
		return
	}
	scanInput := dtos.HFHscanInput{Threshold: 1, BestMatch: false, Root: node}
	response, err := scanner.Scan(&scanInput)
	if err != nil {
		t.Errorf("scannning fails %v", err)
		return
	}

	jsonBytes, _ := json.Marshal(response)
	t.Log(string(jsonBytes))
	expectedResponse := `{"results":[{"path_id":"/root","components":[{"purl":"pkg:github/mirror/busybox","versions":["1_10_3"],"confidence":100}]}]}`
	if string(jsonBytes) != expectedResponse {
		t.Errorf("unexpected response %s. Expected: %s", string(jsonBytes), expectedResponse)
	}
}
