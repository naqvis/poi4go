// Developer: Ali Naqvi
//
// This program or package and any associated files are licensed under the
// Apache License, Version 2.0 (the "License"); you may not use these files
// except in compliance with the License. You can get a copy of the License
// at: http://www.apache.org/licenses/LICENSE-2.0.
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package poifs

import (
	"errors"
	"io"
)

type Entry interface {
	GetName() string
	IsDirectory() bool
	IsDocument() bool
	Parent() DirectoryEntry
	Delete() bool
	RenameTo(newName string) bool
}

type DirectoryEntry interface {
	Entry
	Entries() []Entry
	IsEmpty() bool
	EntryCount() int
	HasEntry(name string) bool
	Entry(name string) (Entry, error)
	StorageClsId() ClassID
	SetStorageClsId(cid ClassID)
	//CreateDocument(name string, stream []byte) DocumentEntry
	CreateDocument(name string, r io.Reader) DocumentEntry
	CreateDirectory(name string) DirectoryEntry
}

type DocumentEntry interface {
	Entry
	GetSize() int
}

type EntryNode struct {
	property IProperty
	parent   *DirectoryNode
}

type DirectoryNode struct {
	*EntryNode
	byname      map[string]Entry
	entries     []Entry
	ofilesystem *POIFSFileSystem
	path        *POIFSDocumentPath
}

type DocumentNode struct {
	*EntryNode
	document *POIFSDocument
}

func NewEntryNode(prop IProperty, parent *DirectoryNode) *EntryNode {
	e := new(EntryNode)
	e.property = prop
	e.parent = parent
	return e
}

func (en *EntryNode) GetProperty() IProperty {
	return en.property
}

func (en *EntryNode) IsRoot() bool {
	return en.parent == nil
}

func (en *EntryNode) GetName() string {
	return en.property.GetName()
}

func (en *EntryNode) IsDirectory() bool {
	return false
}

func (en *EntryNode) IsDocument() bool {
	return false
}

func (en *EntryNode) Parent() DirectoryEntry {
	return en.parent
}
func (en *EntryNode) IsDeleteOK() bool {
	panic("This should never be reached.")
}

func (en *EntryNode) Delete() bool {
	rval := false

	if !en.IsRoot() && en.IsDeleteOK() {
		rval = en.parent.DeleteEntry(en)
	}
	return rval
}

func (en *EntryNode) RenameTo(newName string) bool {
	rval := false
	if !en.IsRoot() {
		rval = en.parent.ChangeName(en.GetName(), newName)
	}
	return rval
}

func NewDirectoryNode(prop ParentProperty, fs *POIFSFileSystem, parent *DirectoryNode) (*DirectoryNode, error) {
	dn := new(DirectoryNode)
	dn.EntryNode = NewEntryNode(prop, parent)
	dn.ofilesystem = fs
	var err error
	if parent == nil {
		dn.path, err = CreatePOIFSDocumentPath()
		if err != nil {
			return nil, err
		}
	} else {
		c := make([]string, 1)
		c[0] = prop.GetName()

		dn.path, err = AddPathToPOIFSDocumentPath(parent.path, c)
		if err != nil {
			return nil, err
		}
	}
	dn.byname = make(map[string]Entry)
	dn.entries = make([]Entry, 0)

	for _, child := range prop.GetChildren() {
		var childNode Entry
		if child.IsDirectory() {
			dir := child.(*DirectoryProperty)
			if dn.ofilesystem != nil {
				childNode, err = NewDirectoryNode(dir, dn.ofilesystem, dn)
				if err != nil {
					return dn, err
				}
			} else {
				panic("DN doesn't have any Filesystem provided.")
			}
		} else {
			childNode = NewDocumentNode(child.(*DocumentProperty), dn)
		}
		dn.entries = append(dn.entries, childNode)
		dn.byname[childNode.GetName()] = childNode
	}

	return dn, nil
}

func (dn *DirectoryNode) GetPath() *POIFSDocumentPath {
	return dn.path
}

func (dn *DirectoryNode) GetFileSystem() *POIFSFileSystem {
	return dn.ofilesystem
}

func (dn *DirectoryNode) ChangeName(oldName, newName string) bool {
	rval := false
	child := (dn.byname[oldName]).(*EntryNode)
	if child != nil {
		rval = dn.GetProperty().(ParentProperty).ChangeName(child.GetProperty(), newName)
		if rval {
			delete(dn.byname, oldName)
			dn.byname[child.GetProperty().GetName()] = child
		}
	}
	return rval
}
func (dn *DirectoryNode) delEntry(entry Entry) {
	found := false
	var idx int
	var e Entry
	for idx, e = range dn.entries {
		if e == entry {
			found = true
			break
		}
	}

	if found {
		//Delete Entry
		dn.entries = append(dn.entries[:idx], dn.entries[idx+1:]...)
		//Avoid potential memory leak, by nil out the pointer values
		dn.entries[len(dn.entries)-1] = nil
		dn.entries = dn.entries[:len(dn.entries)-1]
	}
}

func (dn *DirectoryNode) DeleteEntry(entry *EntryNode) bool {
	rval := dn.GetProperty().(ParentProperty).DelChild(entry.GetProperty())
	if rval {
		dn.delEntry(entry)
		delete(dn.byname, entry.GetName())

		if dn.ofilesystem != nil {
			dn.ofilesystem.Remove(entry)
		}
	}
	return rval
}

func (dn *DirectoryNode) Entries() []Entry {
	return dn.entries[:]
}

func (dn *DirectoryNode) IsEmpty() bool {
	return len(dn.entries) == 0
}

func (dn *DirectoryNode) EntryCount() int {
	return len(dn.entries)
}

func (dn *DirectoryNode) HasEntry(name string) bool {
	if len(name) == 0 {
		return false
	}
	_, ok := dn.byname[name]
	return ok
}

func (dn *DirectoryNode) Entry(name string) (Entry, error) {
	if len(name) == 0 {
		return nil, errors.New("Invalid Entry name provided.")
	}
	rval, ok := dn.byname[name]
	if !ok {
		return nil, errors.New("No such entry: \"" + name + "\"")
	}
	return rval, nil
}

func (dn *DirectoryNode) StorageClsId() ClassID {
	return dn.GetProperty().GetStorageClsid()
}

func (dn *DirectoryNode) SetStorageClsId(cid ClassID) {
	dn.GetProperty().SetStorageClsid(cid)
}

func (dn *DirectoryNode) IsDirectory() bool {
	return true
}

func (dn *DirectoryNode) IsDeleteOK() bool {
	return dn.IsEmpty()
}

func (dn *DirectoryNode) CreateDirectory(name string) DirectoryEntry {
	var rval *DirectoryNode
	var err error
	prop := NewDirProperty(name)
	if dn.ofilesystem != nil {
		rval, err = NewDirectoryNode(prop, dn.ofilesystem, dn)
		if err != nil {
			return nil
		}
		dn.ofilesystem.AddDirectory(prop)
	}
	if rval == nil {
		return nil
	}
	dn.GetProperty().(ParentProperty).AddChild(prop)
	dn.entries = append(dn.entries, rval)
	dn.byname[name] = rval

	return rval
}
func (dn *DirectoryNode) createDocument(doc *POIFSDocument) DocumentEntry {
	var prop *DocumentProperty = doc.GetDocumentProperty()
	var rval *DocumentNode = NewDocumentNode(prop, dn)

	dn.GetProperty().(ParentProperty).AddChild(prop)
	dn.ofilesystem.AddDocument(doc)
	dn.entries = append(dn.entries, rval)
	dn.byname[prop.GetName()] = rval

	return rval
}

//func (dn *DirectoryNode) CreateDocument(name string, stream []byte) DocumentEntry {
func (dn *DirectoryNode) CreateDocument(name string, stream io.Reader) DocumentEntry {
	doc, err := NewPOIFSDocFromStream(name, stream)
	if err != nil {
		return nil
	}
	return dn.createDocument(doc)
}

func NewDocumentNode(prop *DocumentProperty, parent *DirectoryNode) *DocumentNode {
	dn := new(DocumentNode)
	dn.EntryNode = NewEntryNode(prop, parent)
	dn.document = prop.document
	return dn
}

func (dn *DocumentNode) GetDocument() *POIFSDocument {
	return dn.document
}

func (dn *DocumentNode) GetSize() int {
	return dn.GetProperty().GetSize()
}

func (dn *DocumentNode) IsDocument() bool {
	return true
}

func (dn *DocumentNode) IsDeleteOK() bool {
	return true
}
