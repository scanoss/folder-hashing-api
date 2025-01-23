package usecase

import (
	"fmt"
	"path/filepath"
	"strings"

	pb "github.com/scanoss/papi/api/scanningv2"
	m "scanoss.com/hfh-api/pkg/usecase/go-minr-deps/model"
	"scanoss.com/hfh-api/pkg/usecase/go-minr-deps/pkg/filter"
	u "scanoss.com/hfh-api/pkg/usecase/go-minr-deps/utils"
)

type directoryNode struct {
	Path     string
	IsDir    bool
	Children map[string]*directoryNode
	Files    []m.FileItem
}

func newDirectory(path string) *directoryNode {
	return &directoryNode{
		Path:     path,
		IsDir:    true,
		Children: make(map[string]*directoryNode),
		Files:    make([]m.FileItem, 0),
	}
}

func HFHrequestFromPath(path string) *pb.HFHRequest_Children {
	//Init CRC64 table
	m.InitKeyHash()
	rootNode := loadPath(path)
	tree := hashCalcFromNode(rootNode)
	return tree
}

func hashCalcFromNode(dirNode *directoryNode) *pb.HFHRequest_Children {
	hash := HashCalc(dirNode)
	outNode := &pb.HFHRequest_Children{PathId: dirNode.Path,
		SimHashNames:   fmt.Sprintf("%02x", hash.NameHash),
		SimHashContent: fmt.Sprintf("%02x", hash.ContentHash),
		Children:       make([]*pb.HFHRequest_Children, len(dirNode.Children))}

	childNumber := 0
	for _, childNode := range dirNode.Children {
		childHashNode := hashCalcFromNode(childNode)
		outNode.Children[childNumber] = childHashNode
		childNumber++
	}
	return outNode
}

func loadPath(path string) *directoryNode {
	files := u.GetAllFiles(path)
	root := filepath.Clean(path)
	rootParts := strings.Split(root, string(filepath.Separator))
	rootNode := newDirectory(root)

	for f := range files {
		a := filter.EvaluateItem(files[f])
		/*	if !a.Actions.StoreInMZ {
			continue
		}*/
		//log.Printf("%x,%s\n", a.Key, files[f].Name)

		dir := filepath.Dir(a.Path)
		parts := strings.Split(dir, string(filepath.Separator))
		rootNode.Files = append(rootNode.Files, a)

		currentNode := rootNode
		currentPath := root
		for i := len(rootParts); i < len(parts); i++ {
			part := parts[i]
			currentPath = filepath.Join(currentPath, part)

			if _, exists := currentNode.Children[currentPath]; !exists {
				currentNode.Children[currentPath] = newDirectory(currentPath)
			}
			currentNode = currentNode.Children[currentPath]
			currentNode.Files = append(currentNode.Files, a)
		}
	}

	return rootNode
}
