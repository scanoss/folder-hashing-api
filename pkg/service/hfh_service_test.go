// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2018-2022 SCANOSS.COM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	common "github.com/scanoss/papi/api/commonv2"
	pb "github.com/scanoss/papi/api/scanningv2"
	zlog "github.com/scanoss/zap-logging-helper/pkg/logger"
	myconfig "scanoss.com/hfh-api/pkg/config"
	u "scanoss.com/hfh-api/pkg/usecase"
	"scanoss.com/hfh-api/pkg/usecase/ldb"
)

func TestHfhServer_Echo(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := context.Background()
	ctx = ctxzap.ToContext(ctx, zlog.L)
	myConfig, err := myconfig.NewServerConfig(nil)
	if err != nil {
		t.Fatalf("failed to load Config: %v", err)
	}
	s, _ := NewFolderHashingServer(myConfig)

	type args struct {
		ctx context.Context
		req *common.EchoRequest
	}
	tests := []struct {
		name    string
		s       pb.ScanningServer
		args    args
		want    *common.EchoResponse
		wantErr bool
	}{
		{
			name: "Echo",
			s:    s,
			args: args{
				ctx: ctx,
				req: &common.EchoRequest{Message: "Hello there!"},
			},
			want: &common.EchoResponse{Message: "Hello there!"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.Echo(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("service.Echo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("service.Echo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHfhServer_FolderHashScan(t *testing.T) {
	err := zlog.NewSugaredDevLogger()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a sugared logger", err)
	}
	defer zlog.SyncZap()
	ctx := context.Background()
	ctx = ctxzap.ToContext(ctx, zlog.L)
	myConfig, err := myconfig.NewServerConfig(nil)
	if err != nil {
		t.Fatalf("failed to load Config: %v", err)
	}
	s, _ := NewFolderHashingServer(myConfig)
	scannerConfig := u.HFHscanConfig{
		ThStage1:    myConfig.Hfh.Threshold1,
		ThStage2:    myConfig.Hfh.Threshold2,
		ThStage3:    myConfig.Hfh.Threshold3,
		Dmax:        myConfig.Hfh.Dmax,
		SectorTol:   myConfig.Hfh.SectorTol,
		UrlsLimit:   myConfig.Hfh.UrlsLimit,
		HfhTable:    ldb.NewTable("./test/ldb_mock_query_hfh.sh", "test_kb", "hfh", 8, 0, 3, []string{"fileNames", "fileContents", "url"}, ldb.LdbTableDefinitionStandard, false, nil),
		HfhSecTable: ldb.NewTable("./test/ldb_mock_dump_hfhSec.sh", "test_kb", "hfh", 8, 0, 3, []string{"fileNames", "fileContents", "url"}, ldb.LdbTableDefinitionStandard, false, nil),
		UrlTable:    ldb.NewTable("./test/ldb_mock_query_url.sh", "test_kb", "url", 8, 0, 1, []string{"key", "component", "vendor", "version", "date", "license", "purl", "url", "a", "b", "c", "d", "e"}, ldb.LdbTableDefinitionEncrypted, false, nil),
	}
	s.scannerConfig = &scannerConfig

	var hfhRequestData = `{
		"best_match": true,
  		"threshold": 60,
  		"root": {
    		"path_id": "root_folder",
			"sim_hash_names": "6a00168a94ae6238",
    		"sim_hash_content": "6b0a6c14734147e0"
  		}
	}`

	var hfhReq = pb.HFHRequest{}
	err = json.Unmarshal([]byte(hfhRequestData), &hfhReq)
	if err != nil {
		t.Fatalf("an error '%s' was not expected when unmarshalling requestd", err)
	}

	type args struct {
		ctx context.Context
		req *pb.HFHRequest
	}
	tests := []struct {
		name    string
		s       pb.ScanningServer
		args    args
		want    *pb.HFHResponse
		wantErr bool
	}{
		{
			name: "Scan Folder root on children",
			s:    s,
			args: args{
				ctx: ctx,
				req: &hfhReq,
			},
			want: &pb.HFHResponse{Status: &common.StatusResponse{Status: common.StatusCode_SUCCESS, Message: "Success"}},
		},
		{
			name: "Folder Scan for a empty request",
			s:    s,
			args: args{
				ctx: ctx,
				req: &pb.HFHRequest{},
			},
			want:    &pb.HFHResponse{Status: &common.StatusResponse{Status: common.StatusCode_FAILED, Message: "Problems encountered extracting HFH data"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.FolderHashScan(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("service.FolderHashScan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && !reflect.DeepEqual(got.Status, tt.want.Status) {
				t.Errorf("service.FolderHashScan() = %v, want %v", got, tt.want)
			}
		})
	}
}
