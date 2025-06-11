package hfh_cli

import (
	"fmt"
	"path/filepath"
	"strings"

	m "github.com/scanoss/folder-hashing-api/pkg/usecase/examples/hfh_cli/go-minr-deps/model"
	"github.com/scanoss/folder-hashing-api/pkg/usecase/examples/hfh_cli/go-minr-deps/pkg/filter"
	u "github.com/scanoss/folder-hashing-api/pkg/usecase/examples/hfh_cli/go-minr-deps/utils"
	pb "github.com/scanoss/papi/api/scanningv2"
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

func HFHrequestFromPath(path string) (*pb.HFHRequest_Children, error) {
	//Init CRC64 table
	m.InitKeyHash()
	absolutePath, err := filepath.Abs(path)
	rootNode, err := loadPath(absolutePath)
	if err != nil {
		return nil, err
	}
	tree := hashCalcFromNode(rootNode)
	return tree, nil
}

func hashCalcFromNode(dirNode *directoryNode) *pb.HFHRequest_Children {
	hash := HashCalc(dirNode)
	if hash == nil {
		return nil
	}
	outNode := &pb.HFHRequest_Children{
		PathId:          dirNode.Path,
		SimHashNames:    fmt.Sprintf("%016x", hash.NameHash),
		SimHashContent:  fmt.Sprintf("%016x", hash.ContentHash),
		SimHashDirNames: fmt.Sprintf("%016x", hash.DirHash),
		Children:        make([]*pb.HFHRequest_Children, 0),
		LangExtensions:  make(map[string]int32),
	}
	for k, v := range hash.LanguageExtensions {
		outNode.LangExtensions[k] = int32(v)
	}
	for _, childNode := range dirNode.Children {
		childHashNode := hashCalcFromNode(childNode)
		if childHashNode == nil {
			continue
		}
		outNode.Children = append(outNode.Children, childHashNode)
	}
	return outNode
}

func loadPath(path string) (*directoryNode, error) {
	files := u.GetAllFiles(path)
	if files == nil {
		return nil, fmt.Errorf("invalid or empty directory")
	}
	root := filepath.Clean(path)
	rootParts := strings.Split(root, string(filepath.Separator))
	rootNode := newDirectory(root)

	for f := range files {
		a := filter.EvaluateItem(files[f])
		if !a.Actions.StoreInFile || a.Actions.CompletelyIgnore {
			continue
		}
		if !ShouldAcceptPath(a.Path) {
			continue
		}
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

	return rootNode, nil
}
