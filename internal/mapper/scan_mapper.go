// Package mapper provides conversion between protobuf and domain models.
package mapper

import (
	"github.com/scanoss/papi/api/scanningv2"

	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
)

// ScanMapper converts between protobuf and domain scan models.
type ScanMapper interface {
	ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest
	DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse
	ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode
}
