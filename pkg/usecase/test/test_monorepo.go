package test

import (
	pb "github.com/scanoss/papi/api/scanningv2"
)

var Monorepo_root = &pb.HFHRequest_Children{
	PathId:         "/monorepo",
	SimHashNames:   "81517c5492aa0fc8",
	SimHashContent: "f98fc3f728a8b4d4",
	Children: []*pb.HFHRequest_Children{
		{
			PathId:         "/monorepo/deps",
			SimHashNames:   "7e515d5592ae0bc8",
			SimHashContent: "d98fc3f520e83444",
			Children: []*pb.HFHRequest_Children{
				{
					PathId:         "/monorepo/deps/libsignal-protocol-test",
					SimHashNames:   "7253dc5d9e794d5a",
					SimHashContent: "fbbbcaf62ae01ce0",
					Children: []*pb.HFHRequest_Children{
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/android",
							SimHashNames:   "7a94747a58c053e6",
							SimHashContent: "f3c2fe0f9fdfff7c",
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/java",
							SimHashNames:   "8b53dc6dde6b4b52",
							SimHashContent: "fbabc3f6a8f03ce0",
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/protobuf",
							SimHashNames:   "cabf3fcbd1fffff9",
							SimHashContent: "e83dfde7176c9eff",
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/tests",
							SimHashNames:   "7d11957bd77a3dcc",
							SimHashContent: "9bda46797ac28afa",
						},
					},
				},
				{
					PathId:         "/monorepo/deps/other",
					SimHashNames:   "60f56d190b871b42",
					SimHashContent: "7c3b5bd6a67a7458",
					Children: []*pb.HFHRequest_Children{
						{
							PathId:         "/monorepo/deps/other/recastnavigation-1.6.0",
							SimHashNames:   "60f56d190b871b42",
							SimHashContent: "7c3b5bd6a67a7458",
						},
					},
				},
				{
					PathId:         "/monorepo/deps/zlib-1.2.13",
					SimHashNames:   "841b7c7791ea27e8",
					SimHashContent: "808ec3a921ada497",
					Children: []*pb.HFHRequest_Children{
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/amiga",
							SimHashNames:   "80dfed122852b8ac",
							SimHashContent: "7f4ffffff53ffef7",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/contrib",
							SimHashNames:   "7f1b785295ca866a",
							SimHashContent: "c0cfdbe84d897497",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/examples",
							SimHashNames:   "b6e7b445e7cea4bf",
							SimHashContent: "f69eec852175f7d1",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/msdos",
							SimHashNames:   "9af6ddf7b62e1eff",
							SimHashContent: "a20a59710ce899c",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/nintendods",
							SimHashNames:   "82f56db23c448aa9",
							SimHashContent: "6f830d8e90ebd5d3",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/old",
							SimHashNames:   "c7f4ed75bccf8eef",
							SimHashContent: "911893bd20479514",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/os400",
							SimHashNames:   "b9fef7cfb6ef3f3d",
							SimHashContent: "3ddf81f356752eb3",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/qnx",
							SimHashNames:   "a3aebf18efb23ebf",
							SimHashContent: "9f450a908a4ac50f",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/test",
							SimHashNames:   "aceeb5cce72ff33d",
							SimHashContent: "20e2521d626448eb",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/watcom",
							SimHashNames:   "93f1e79c780d60fc",
							SimHashContent: "fbafff5fff1fbd7e",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/win32",
							SimHashNames:   "84ff7f27b4649ace",
							SimHashContent: "9ceed789b1349654",
						},
					},
				},
			},
		},
		{
			PathId:         "/monorepo/other",
			SimHashNames:   "91cf9a9c892c17fe",
			SimHashContent: "bba7faff6c8abfbf",
			Children: []*pb.HFHRequest_Children{
				{
					PathId:         "/monorepo/other/CSerial-0.3_test",
					SimHashNames:   "99db919561663ef0",
					SimHashContent: "b7c278af951aec17",
					Children: []*pb.HFHRequest_Children{
						{
							PathId:         "/monorepo/other/CSerial-0.3_test/debian",
							SimHashNames:   "8ed3991d6ba20ed0",
							SimHashContent: "3bcbf8efb716ee37",
						},
						{
							PathId:         "/monorepo/other/CSerial-0.3_test/examples",
							SimHashNames:   "c1e3fdcd8fa5bfbf",
							SimHashContent: "86eb0475e01d55b5",
						},
					},
				},
				{
					PathId:         "/monorepo/other/rapidjson-1.1.0-test",
					SimHashNames:   "788dae1ddd6b737f",
					SimHashContent: "3937f3d66ca89ffc",
					Children: []*pb.HFHRequest_Children{
						{
							PathId:         "/monorepo/other/rapidjson-1.1.0-test/bin",
							SimHashNames:   "606b1d4755a08512",
							SimHashContent: "fbfdf6f7eaf9fffc",
						},
						{
							PathId:         "/monorepo/other/rapidjson-1.1.0-test/docker",
							SimHashNames:   "75ad55386e099693",
							SimHashContent: "3534a3a2871915ec",
						},
						{
							PathId:         "/monorepo/other/rapidjson-1.1.0-test/include",
							SimHashNames:   "78b5aa1ff925737f",
							SimHashContent: "bc17fbd76caa9fbf",
						},
					},
				},
			},
		},
	},
}
