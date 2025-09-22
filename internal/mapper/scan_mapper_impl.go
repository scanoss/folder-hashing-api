package mapper

import (
	"maps"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/papi/api/scanningv2"
)

type ScanMapperImpl struct{}

func NewScanMapper() ScanMapper {
	return &ScanMapperImpl{}
}

// ProtoToDomain converts protobuf HFHRequest to domain ScanRequest
func (m *ScanMapperImpl) ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest {
	if req == nil {
		return nil
	}

	return &entities.ScanRequest{
		RankThreshold:      int(req.RankThreshold),
		RecursiveThreshold: req.RecursiveThreshold,
		Category:           req.Category,
		QueryLimit:         int(req.QueryLimit),
		Root:               m.ChildrenToDomain(req.Root),
	}
}

// DomainToProto converts domain ScanResponse to protobuf HFHResponse
func (m *ScanMapperImpl) DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse {
	if resp == nil {
		return nil
	}

	protoResp := &scanningv2.HFHResponse{}

	// Convert results
	if len(resp.Results) > 0 {
		protoResp.Results = make([]*scanningv2.HFHResponse_Result, len(resp.Results))
		for i, result := range resp.Results {
			protoResp.Results[i] = m.scanResultToProto(result)
		}
	}

	return protoResp
}

// ChildrenToDomain converts protobuf Children to domain FolderNode
func (m *ScanMapperImpl) ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode {
	if children == nil {
		return nil
	}

	// Convert language extensions
	langExt := make(entities.LanguageExtensions)
	maps.Copy(langExt, children.LangExtensions)

	node := &entities.FolderNode{
		PathID:          children.PathId,
		SimHashNames:    children.SimHashNames,
		SimHashContent:  children.SimHashContent,
		SimHashDirNames: children.SimHashDirNames,
		LangExtensions:  langExt,
	}

	// Convert children recursively
	if len(children.Children) > 0 {
		node.Children = make([]*entities.FolderNode, len(children.Children))
		for i, child := range children.Children {
			node.Children[i] = m.ChildrenToDomain(child)
		}
	}

	return node
}

// scanResultToProto converts domain ScanResult to protobuf Result
func (m *ScanMapperImpl) scanResultToProto(result *entities.ScanResult) *scanningv2.HFHResponse_Result {
	if result == nil {
		return nil
	}

	protoResult := &scanningv2.HFHResponse_Result{
		PathId: result.PathID,
	}

	for _, group := range result.ComponentGroups {
		var versions []*scanningv2.HFHResponse_Version
		for _, v := range group.Versions {
			versions = append(versions, &scanningv2.HFHResponse_Version{
				Version: v.Version,
				Score:   v.Score,
			})
		}

		protoResult.Components = append(protoResult.Components, &scanningv2.HFHResponse_Component{
			Purl:     group.PURL,
			Name:     group.Name,
			Vendor:   group.Vendor,
			Versions: versions,
			Rank:     group.Rank,
			Order:    group.Order,
		})
	}

	return protoResult
}
