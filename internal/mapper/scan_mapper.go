package mapper

import (
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
