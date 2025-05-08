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
	mv "scanoss.com/hfh-api/pkg/usecase/milvus"
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

	config, err := myconfig.NewServerConfig(nil)
	if err != nil {
		return nil, fmt.Errorf("Fatal error loading default config")
	}

	db, err := mv.NewMilvusDb("", "", "test")

	if err != nil {
		return nil, fmt.Errorf("Failed to initializate milvus kb: %v. If this is the fisrt time you run this test, please run milvus_deploy_script.sh from pkg/usecase/milvus and try again", err)
	}

	scannerConfig := HFHscanConfig{
		ThStage1:  config.Hfh.Threshold1,
		ThStage2:  config.Hfh.Threshold2,
		ThStage3:  config.Hfh.Threshold3,
		Dmax:      config.Hfh.Dmax,
		UrlsLimit: config.Hfh.UrlsLimit,
		mvDb:      db,
	}
	dt := dtos.HFHscanInput{BestMatch: true, Threshold: 100}
	scanner := HFHScanNew(s, &scannerConfig, &dt)
	return scanner, nil
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
		SimHashNames:   "8b2dd17b4ed2a3c",
		SimHashContent: "49c2ed35d931fee9",
		Children: []*dtos.HFHScanInputChildren{
			{
				PathId:         "/root/child1",
				SimHashNames:   "98b2dd17b4ed2a2c",
				SimHashContent: "58cbcc9d9835dbdb",
			},
			{
				PathId:         "/root/child2",
				SimHashNames:   "891b481f843a2206",
				SimHashContent: "b8e2fa2dcb6a3e56",
			},
		},
	}
	err = scanner.scanTreeFirstStage(node)
	if err != nil {
		t.Errorf("unexpected error during scan process %v", err)
		return
	}
	t.Log(scanner.resultsMap)
	expectedPurl := "pkg:gnu/time"
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

func TestHFHthirdStep(t *testing.T) {
	resultsMap := map[string]HFHscanResult{
		"/monorepo/deps/libsignal-protocol-test/gradle": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/signalapp/libsignal-protocol-java",
					Versions: []string{"v2.3.0"},
					Rank:     2,
					Date:     time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 100,
		},
		"/monorepo/deps/libsignal-protocol-test/java/src": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/signalapp/libsignal-protocol-java",
					Versions: []string{"v2.3.0"},
					Rank:     2,
					Date:     time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 95.83333,
		},
		"/monorepo/deps/libsignal-protocol-test/tests/src": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/signalapp/libsignal-protocol-java",
					Versions: []string{"v2.3.0"},
					Rank:     100,
					Date:     time.Date(2016, 10, 18, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 100,
		},
		"/monorepo/deps/other": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/recastnavigation/recastnavigation",
					Versions: []string{"v1.6.0"},
					Rank:     100,
					Date:     time.Date(2023, 5, 21, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       1,
			Probability: 100,
		},
		"/monorepo/deps/zlib-1.2.13": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:gitlab/rluna-database/nosql/arangodb/arangodb-2020",
					Versions: []string{"v3.11.3.1"},
					Rank:     100,
					Date:     time.Date(2023, 9, 27, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 87.5,
		},
		"/monorepo/other/rapidjson-1.1.0-test": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/nilsbore/roswasm_suite",
					Versions: []string{"release-8"},
					Rank:     100,
					Date:     time.Date(2021, 3, 11, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       2,
			Probability: 75,
		},
		"/monorepo/other/rapidjson-1.1.0-test/bin": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/rhysd/tinyjson",
					Versions: []string{"v1.0.0"},
					Rank:     100,
					Date:     time.Date(2020, 4, 28, 0, 0, 0, 0, time.UTC),
				},
			},
			Stage:       0,
			Probability: 41.666664,
		},
		"/monorepo/other/rapidjson-1.1.0-test/docker": {
			Components: []HFHscanResultItem{
				{
					Purl:     "pkg:github/jmenga/docker-ansible",
					Versions: []string{"module-3-after"},
					Rank:     100,
					Date:     time.Date(2016, 1, 29, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:github/podbox/docker-teamcity",
					Versions: []string{"2e44b32"},
					Rank:     100,
					Date:     time.Date(2016, 5, 6, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:bitbucket/suprocktech/docker_cortex_m",
					Versions: []string{"1.0.0"},
					Rank:     100,
					Date:     time.Date(2017, 9, 12, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:gitlab/gableroux/gitlab-ci-example-docker",
					Versions: []string{"d57a02c7"},
					Rank:     100,
					Date:     time.Date(2018, 1, 17, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:bitbucket/wprowebdev/ubuntu-node-gulp",
					Versions: []string{"v1.0.1"},
					Rank:     100,
					Date:     time.Date(2018, 8, 17, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:bitbucket/sarkisv/docker-git-test",
					Versions: []string{"v4.1.0"},
					Rank:     100,
					Date:     time.Date(2019, 6, 23, 0, 0, 0, 0, time.UTC),
				},
				// Continued with remaining docker components...
				{
					Purl:     "pkg:github/morphy2k/docker-hello-world",
					Versions: []string{"v0.1.0-beta.5"},
					Rank:     100,
					Date:     time.Date(2024, 10, 10, 0, 0, 0, 0, time.UTC),
				},
				{
					Purl:     "pkg:github/mohrm/traktorgaehn",
					Versions: []string{"v0.2.0-beta4"},
					Rank:     100,
					Date:     time.Date(2024, 11, 27, 0, 0, 0, 0, time.UTC),
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
	scanInput := dtos.HFHscanInput{Root: test.Monorepo_root}
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
	var expectedProb float32 = 82.3
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
	expectedResponse := `{"/monorepo":{"components":[],"Stage":2,"probability":66.666664},"/monorepo/deps/libsignal-protocol-test/java":{"components":[{"purl":"pkg:github/signalapp/libsignal-protocol-java","versions":["v2.3.0"],"rank":0,"date":"0001-01-01T00:00:00Z"}],"Stage":1,"probability":83.33333},"/monorepo/deps/other":{"components":[{"purl":"pkg:github/recastnavigation/recastnavigation","versions":["v1.6.0"],"rank":0,"date":"0001-01-01T00:00:00Z"}],"Stage":1,"probability":100},"/monorepo/other/CSerial-0.3_test":{"components":[{"purl":"pkg:github/rm5248/cserial","versions":["v0.3","7b5cbd5"],"rank":0,"date":"0001-01-01T00:00:00Z"}],"Stage":0,"probability":50}}`
	if string(jsonBytes) != expectedResponse {
		t.Errorf("unexpected response %s. Expected: %s", string(jsonBytes), expectedResponse)
	}
}
func TestHFHScan(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Logf("an error '%s' was not expected when opening a sugared logger", err)
	}

	defer zlog.SyncZap()
	ctx := ctxzap.ToContext(context.Background(), zlog.L)
	s := ctxzap.Extract(ctx).Sugar()

	cfg, err := myconfig.NewServerConfig(nil)
	if err != nil {
		t.Errorf("Fatal error loading default config")
	}

	scannerConfig := HFHScanInit(cfg, true)
	if scannerConfig == nil {
		t.Skipf("scan failed during initialization. To tun this test be sure that you have a valid kb instaled")
		return
	}

	scanInput := dtos.HFHscanInput{Threshold: 100.0, BestMatch: false, Root: test.Monorepo_root}
	scanner := HFHScanNew(s, scannerConfig, &scanInput)
	response, err := scanner.Scan(scanInput.Root)
	if err != nil {
		t.Errorf("scanning fails %v", err)
		return
	}
	jsonBytes, _ := json.Marshal(response)
	t.Log(string(jsonBytes))
	expectedResponse := `{"results":[{"path_id":"/monorepo/deps/libsignal-protocol-test","components":[{"purl":"pkg:github/signalapp/libsignal-protocol-java","versions":["v2.3.0"],"rank":4}],"stage":1,"probability":73.333336},{"path_id":"/monorepo/deps/other","components":[{"purl":"pkg:github/recastnavigation/recastnavigation","versions":["v1.6.0","77f7e54"],"rank":3}],"stage":1,"probability":63.333332}]}`
	if string(jsonBytes) != expectedResponse {
		t.Errorf("unexpected response %s. Expected: %s", string(jsonBytes), expectedResponse)
	}
}
