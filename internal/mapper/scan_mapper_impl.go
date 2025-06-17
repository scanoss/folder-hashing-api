package mapper

import (
	"maps"
	"sort"

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

	// Collect all version results from all component groups
	var allVersionResults []entities.VersionResult
	for _, group := range result.ComponentGroups {
		if group != nil {
			allVersionResults = append(allVersionResults, group.AllVersions...)
		}
	}

	// Sort by score (higher score is better)
	sort.Slice(allVersionResults, func(i, j int) bool {
		return allVersionResults[i].Score > allVersionResults[j].Score
	})

	// Group by PURL and track the best score for each PURL
	purlMap := make(map[string][]entities.VersionResult)
	purlScoreMap := make(map[string]float32)

	// Group versions by PURL and track the best score for each PURL
	for _, versionResult := range allVersionResults {
		if versionResult.Metadata != nil && versionResult.Metadata["purl"] != nil {
			purl := versionResult.Metadata["purl"].(string)
			purlMap[purl] = append(purlMap[purl], versionResult)

			// Set score for this PURL if not already set (first occurrence has best score)
			if _, exists := purlScoreMap[purl]; !exists {
				purlScoreMap[purl] = versionResult.Score
			}
		}
	}

	// Create a slice of PURLs with their scores for sorting
	type purlWithScore struct {
		purl  string
		score float32
	}

	purlScores := make([]purlWithScore, 0, len(purlMap))
	for purl, score := range purlScoreMap {
		purlScores = append(purlScores, purlWithScore{purl, score})
	}

	// Sort PURLs by score (higher scores first)
	sort.Slice(purlScores, func(i, j int) bool {
		return purlScores[i].score > purlScores[j].score
	})

	// Create components with sequential ranks
	components := make([]*scanningv2.HFHResponse_Component, len(purlScores))
	for i, ps := range purlScores {
		versions := purlMap[ps.purl]

		// Sort versions within this component by score (higher scores first)
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].Score > versions[j].Score
		})

		// Extract all version strings in score-sorted order
		versionStrings := make([]string, len(versions))
		for j, v := range versions {
			versionStrings[j] = v.Version
		}

		// Create component with sequential rank
		components[i] = &scanningv2.HFHResponse_Component{
			Purl:     ps.purl,
			Versions: versionStrings,
			Rank:     int32(i + 1), // Sequential rank (1, 2, 3, ...)
		}
	}

	protoResult.Components = components
	return protoResult
}
