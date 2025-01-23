package test

import (
	"scanoss.com/hfh-api/pkg/dtos"
)

var Monorepo_root = &dtos.HFHScanInputChildren{
	PathId:         "/monorepo",
	SimHashNames:   "659d18058f6f0fca",
	SimHashContent: "da8ff2b524e23c84",
	Children: []*dtos.HFHScanInputChildren{
		{
			PathId:         "/monorepo/deps",
			SimHashNames:   "7453d94582af2548",
			SimHashContent: "c88cc2f420f43444",
			Children: []*dtos.HFHScanInputChildren{
				{
					PathId:         "/monorepo/deps/libsignal-protocol-test",
					SimHashNames:   "7a53cd7c9e79495a",
					SimHashContent: "dbabcae5aae83ce0",
					Children: []*dtos.HFHScanInputChildren{
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/android",
							SimHashNames:   "66c51a1814c76dc5",
							SimHashContent: "ffeafe299f5ff939",
							Children: []*dtos.HFHScanInputChildren{
								{
									PathId:         "/monorepo/deps/libsignal-protocol-test/android/src",
									SimHashNames:   "a8d9f77f1cc577f5",
									SimHashContent: "e380fe089f5df130",
								},
							},
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/gradle",
							SimHashNames:   "9753eac8a64833b2",
							SimHashContent: "c7d9ffbffbffbf7d",
							Children: []*dtos.HFHScanInputChildren{
								{
									PathId:         "/monorepo/deps/libsignal-protocol-test/gradle/wrapper",
									SimHashNames:   "9753eac8a64833b2",
									SimHashContent: "c7d9ffbffbffbf7d",
								},
							},
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/java",
							SimHashNames:   "8653d84ddc6b4952",
							SimHashContent: "fbafcbf6aaf87ce4",
							Children: []*dtos.HFHScanInputChildren{
								{
									PathId:         "/monorepo/deps/libsignal-protocol-test/java/src",
									SimHashNames:   "8b53dc6dde6b4b52",
									SimHashContent: "fbabc3f6a8f03ce0",
								},
							},
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/protobuf",
							SimHashNames:   "cabf3fcbd1fffff9",
							SimHashContent: "e83dfde7176c9eff",
						},
						{
							PathId:         "/monorepo/deps/libsignal-protocol-test/tests",
							SimHashNames:   "8831917b976abdcc",
							SimHashContent: "1b5a42496ac08aea",
							Children: []*dtos.HFHScanInputChildren{
								{
									PathId:         "/monorepo/deps/libsignal-protocol-test/tests/src",
									SimHashNames:   "7d11957bd77a3dcc",
									SimHashContent: "9bda46797ac28afa",
								},
							},
						},
					},
				},
				{
					PathId:         "/monorepo/deps/other",
					SimHashNames:   "5f3dcd05ab272b52",
					SimHashContent: "643a57d3b6f27064",
					Children: []*dtos.HFHScanInputChildren{
						{
							PathId:         "/monorepo/deps/other/recastnavigation-1.6.0",
							SimHashNames:   "5f3dcd05ab272b52",
							SimHashContent: "643a57d3b6f27064",
							Children: []*dtos.HFHScanInputChildren{
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/DebugUtils",
									SimHashNames:   "8d73dfec7e9fb727",
									SimHashContent: "4ab27756da7bec6c",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/Detour",
									SimHashNames:   "9be5af29fa0f73eb",
									SimHashContent: "4fabf920800f6dc",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/DetourCrowd",
									SimHashNames:   "74243ddda8de2b73",
									SimHashContent: "2911da93a57c7240",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/DetourTileCache",
									SimHashNames:   "b76adee84ecaf4bf",
									SimHashContent: "8b4b99f0a9a4c7ed",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/Docs",
									SimHashNames:   "775c095780b58a42",
									SimHashContent: "645ad6b1beb7f129",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/Recast",
									SimHashNames:   "83adaf31ef486e4c",
									SimHashContent: "5b746920d8ff3538",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/RecastDemo",
									SimHashNames:   "c39cfe996bb7ffce",
									SimHashContent: "f5b312feb65b5044",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/Tests",
									SimHashNames:   "a2e75b78ebcbef39",
									SimHashContent: "e401292794923095",
								},
								{
									PathId:         "/monorepo/deps/other/recastnavigation-1.6.0/.github",
									SimHashNames:   "ffffffffffffffff",
									SimHashContent: "ffffffffffffffff",
								},
							},
						},
					},
				},
				{
					PathId:         "/monorepo/deps/zlib-1.2.13",
					SimHashNames:   "9592d05593ca25e8",
					SimHashContent: "808cd3f839ad8497",
					Children: []*dtos.HFHScanInputChildren{
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/amiga",
							SimHashNames:   "80dfed122852b8ac",
							SimHashContent: "7f4ffffff53ffef7",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/contrib",
							SimHashNames:   "9b9e585395ca84e8",
							SimHashContent: "c0cdd2e84d892087",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/doc",
							SimHashNames:   "c5ea999de7a197eb",
							SimHashContent: "e6b02ffdff9f77eb",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/examples",
							SimHashNames:   "c2e7b557ffaef5bf",
							SimHashContent: "369ecc852175b7d0",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/msdos",
							SimHashNames:   "9af6ddf7b62e1eff",
							SimHashContent: "a20a59710ce899c",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/nintendods",
							SimHashNames:   "84ac2dfa5c886133",
							SimHashContent: "ffa7bfbfbafbffd7",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/old",
							SimHashNames:   "b472edf0ba96ae9d",
							SimHashContent: "809892bde0c79704",
						},
						{
							PathId:         "/monorepo/deps/zlib-1.2.13/os400",
							SimHashNames:   "a2d8e3986ba8a037",
							SimHashContent: "fddfd3f3577f6ef3",
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
							SimHashNames:   "99b6669cb47637de",
							SimHashContent: "bcffff89f9329fde",
						},
					},
				},
			},
		},
		{
			PathId:         "/monorepo/other",
			SimHashNames:   "59bc10814f6d0e86",
			SimHashContent: "9a837baf7c427aae",
			Children: []*dtos.HFHScanInputChildren{
				{
					PathId:         "/monorepo/other/CSerial-0.3_test",
					SimHashNames:   "a37299a5ebe30eba",
					SimHashContent: "b7c378ab851bec97",
					Children: []*dtos.HFHScanInputChildren{
						{
							PathId:         "/monorepo/other/CSerial-0.3_test/debian",
							SimHashNames:   "8cc39b1d6b228ed2",
							SimHashContent: "23c8e8eb1412ce17",
						},
						{
							PathId:         "/monorepo/other/CSerial-0.3_test/examples",
							SimHashNames:   "66433be9e6087739",
							SimHashContent: "f7efdc75f21f77ff",
						},
					},
				},
				{
					PathId:         "/monorepo/other/rapidjson-1.1.0-test",
					SimHashNames:   "52bd16810f6d0e84",
					SimHashContent: "9ab33b8d7ce25aae",
					Children: []*dtos.HFHScanInputChildren{
						{
							PathId:         "/monorepo/other/rapidjson-1.1.0-test/CMakeModules",
							SimHashNames:   "d9fe9bf3ddd7bfdf",
							SimHashContent: "fa4253ba414ada99",
						},
						{
							PathId:         "/monorepo/other/rapidjson-1.1.0-test/bin",
							SimHashNames:   "59bd10814f6d0e84",
							SimHashContent: "9aab3b897ce2688e",
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
