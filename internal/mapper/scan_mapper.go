package mapper

import (
	"maps"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/papi/api/scanningv2"
)

// ScanMapper defines the interface for mapping between protobuf and domain entities
type ScanMapper interface {
	// ProtoToDomain converts protobuf HFHRequest to domain ScanRequest
	ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest

	// DomainToProto converts domain ScanResponse to protobuf HFHResponse
	DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse

	// ChildrenToDomain converts protobuf Children to domain FolderNode
	ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode

	// ComponentGroupToResult converts domain ComponentGroup to protobuf Result
	ComponentGroupToResult(group *entities.ComponentGroup, pathID string) *scanningv2.HFHResponse_Result
}

// scanMapper implements the ScanMapper interface
type scanMapper struct{}

// NewScanMapper creates a new scan mapper
func NewScanMapper() ScanMapper {
	return &scanMapper{}
}

// ProtoToDomain converts protobuf HFHRequest to domain ScanRequest
func (m *scanMapper) ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest {
	if req == nil {
		return nil
	}

	return &entities.ScanRequest{
		BestMatch: req.BestMatch,
		Threshold: req.Threshold,
		Root:      m.ChildrenToDomain(req.Root),
	}
}

// DomainToProto converts domain ScanResponse to protobuf HFHResponse
func (m *scanMapper) DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse {
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
func (m *scanMapper) ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode {
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
func (m *scanMapper) ComponentGroupToResult(group *entities.ComponentGroup, pathID string) *scanningv2.HFHResponse_Result {
	if group == nil {
		return nil
	}

	result := &scanningv2.HFHResponse_Result{
		PathId:      pathID,
		Probability: group.BestMatch.Score,
		Stage:       1, // Default stage
	}

	// Convert component group to protobuf components
	// Note: The current protobuf structure uses a flat list of components
	// We need to flatten our ComponentGroup structure
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
func (m *scanMapper) scanResultToProto(result *entities.ScanResult) *scanningv2.HFHResponse_Result {
	if result == nil {
		return nil
	}

	protoResult := &scanningv2.HFHResponse_Result{
		PathId:      result.PathID,
		Probability: result.Probability,
		Stage:       result.Stage,
	}

	// Convert component groups to flat component list
	var allComponents []*scanningv2.HFHResponse_Component

	for _, group := range result.ComponentGroups {
		// Convert each component group to components
		groupResult := m.ComponentGroupToResult(group, result.PathID)
		if groupResult != nil && len(groupResult.Components) > 0 {
			allComponents = append(allComponents, groupResult.Components...)
		}
	}

	protoResult.Components = allComponents
	return protoResult
}
