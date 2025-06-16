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
		RankThreshold: req.RankThreshold,
		Category:      req.Category,
		QueryLimit:    req.QueryLimit,
		Root:          m.ChildrenToDomain(req.Root),
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

	// Convert status
	if resp.Status != nil {
		// Note: We need to map to the common status response from papi
		// This might need adjustment based on the actual papi structure
		// For now, we'll create a basic mapping
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

// ComponentGroupToResult converts domain ComponentGroup to protobuf Result
func (m *ScanMapperImpl) ComponentGroupToResult(group *entities.ComponentGroup, pathID string) *scanningv2.HFHResponse_Result {
	if group == nil {
		return nil
	}

	result := &scanningv2.HFHResponse_Result{
		PathId: pathID,
	}

	var components []*scanningv2.HFHResponse_Component

	// Add best match
	bestMatchComponent := &scanningv2.HFHResponse_Component{
		Purl:     group.BestMatch.Metadata["purl"].(string),
		Versions: []string{group.BestMatch.Version},
		Rank:     1, // Best match gets rank 1
	}
	components = append(components, bestMatchComponent)

	// Add other versions if they exist
	for i, version := range group.AllVersions {
		if i == 0 {
			continue // Skip best match as we already added it
		}

		component := &scanningv2.HFHResponse_Component{
			Purl:     version.Metadata["purl"].(string),
			Versions: []string{version.Version},
			Rank:     int32(i + 1), // Rank based on position
		}
		components = append(components, component)
	}

	result.Components = components
	return result
}

// scanResultToProto converts domain ScanResult to protobuf Result
func (m *ScanMapperImpl) scanResultToProto(result *entities.ScanResult) *scanningv2.HFHResponse_Result {
	if result == nil {
		return nil
	}

	protoResult := &scanningv2.HFHResponse_Result{
		PathId: result.PathID,
	}

	// Collect all version results from all component groups
	var allVersionResults []entities.VersionResult
	for _, group := range result.ComponentGroups {
		if group != nil {
			allVersionResults = append(allVersionResults, group.AllVersions...)
		}
	}

	// Sort by score (descending - best to worst)
	for i := 0; i < len(allVersionResults)-1; i++ {
		for j := i + 1; j < len(allVersionResults); j++ {
			if allVersionResults[i].Score < allVersionResults[j].Score {
				allVersionResults[i], allVersionResults[j] = allVersionResults[j], allVersionResults[i]
			}
		}
	}

	// Convert each version result to a separate component
	var allComponents []*scanningv2.HFHResponse_Component
	for i, versionResult := range allVersionResults {
		if versionResult.Metadata != nil && versionResult.Metadata["purl"] != nil {
			component := &scanningv2.HFHResponse_Component{
				Purl:     versionResult.Metadata["purl"].(string),
				Versions: []string{versionResult.Version},
				Rank:     int32(i + 1),
			}
			allComponents = append(allComponents, component)
		}
	}

	protoResult.Components = allComponents
	return protoResult
}
