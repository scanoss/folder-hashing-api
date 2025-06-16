package mapper

import (
	"github.com/scanoss/folder-hashing-api/internal/domain/entities"
	"github.com/scanoss/papi/api/scanningv2"
)

type ScanMapper interface {
	ProtoToDomain(req *scanningv2.HFHRequest) *entities.ScanRequest
	DomainToProto(resp *entities.ScanResponse) *scanningv2.HFHResponse
	ChildrenToDomain(children *scanningv2.HFHRequest_Children) *entities.FolderNode
}
