package usecase

import (
	m "scanoss.com/hfh-api/pkg/usecase/go-minr-deps/model"
)

type DirectoryNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children map[string]*DirectoryNode
	Files    []m.FileItem
}

func NewDirectory(name, path string) *DirectoryNode {
	return &DirectoryNode{
		Name:     name,
		Path:     path,
		IsDir:    true,
		Children: make(map[string]*DirectoryNode),
		Files:    make([]m.FileItem, 0),
	}
}

func (d *DirectoryNode) AddFile(file m.FileItem) {
	d.Files = append(d.Files, file)
}
