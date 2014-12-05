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
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
)

var (
	_max_name_length int = _name_size_offset/SHORT_SIZE - 1
)

const (
	_default_fill     byte = 0x00
	_name_size_offset int  = 0x40
	_NO_INDEX              = -1

	// useful offsets
	_node_color_offset        = 0x43
	_previous_property_offset = 0x44
	_next_property_offset     = 0x48
	_child_property_offset    = 0x4C
	_storage_clsid_offset     = 0x50
	_user_flags_offset        = 0x60
	_seconds_1_offset         = 0x64
	_days_1_offset            = 0x68
	_seconds_2_offset         = 0x6C
	_days_2_offset            = 0x70
	_start_block_offset       = 0x74
	_size_offset              = 0x78

	// node colors
	_NODE_BLACK byte = 1
	_NODE_RED   byte = 0

	_big_block_minimum_bytes = BIG_BLOCK_MINIMUM_DOCUMENT_SIZE

	PROPERTY_TYPE_OFFSET int = 0x42
	DIRECTORY_TYPE           = 1
	DOCUMENT_TYPE            = 2
	ROOT_TYPE                = 5
)

//returns negative if p1 < p2
// zero if p1 == p2
// positive if p1 > p2
type Comparator func(p1, p2 IProperty) int

//PropertySorter implements the Sort interface, Sorting the changes within
type PropertyComparator struct {
	properties []IProperty
	compare    Comparator
}

// Sort sorts the argument slice according to the less functions passed to PropertySorter.
func (pc *PropertyComparator) Sort(props []IProperty) {
	pc.properties = props
	sort.Sort(pc)
}

//returns a Comparator that sorts using the compare function
func PropertySorter(compare Comparator) *PropertyComparator {
	return &PropertyComparator{compare: compare}
}

// Len is part of sort.Interface.
func (pc *PropertyComparator) Len() int {
	return len(pc.properties)
}

// Swap is part of sort.Interface.
func (pc *PropertyComparator) Swap(i, j int) {
	pc.properties[i], pc.properties[j] = pc.properties[j], pc.properties[i]
}

// Less is part of sort.Interface.
func (pc *PropertyComparator) Less(i, j int) bool {
	p, q := pc.properties[i], pc.properties[j]
	k := pc.compare(p, q)
	if k < 0 {
		return true
	} else if k > 0 {
		return false
	} else {
		return false
	}
}

type Child interface {
	GetNextChild() Child
	GetPreviousChild() Child
	SetPreviousChild(child Child)
	SetNextChild(child Child)
}

type Parent interface {
	//Child
	AddChild(prop IProperty) error
	DelChild(prop IProperty) bool
	GetChildren() []IProperty
	ChangeName(prop IProperty, newName string) bool
}

/*
type ChildProperty interface {
	IProperty
	Child
}
*/
type ParentProperty interface {
	IProperty
	Parent
}

type IProperty interface {
	Child
	GetSize() int
	SetSize(size int)
	SetNodeColor(nodeColor byte)
	SetPropertyType(propertyType byte)
	SetStartBlock(startBlock int)
	IsDirectory() bool
	GetName() string
	SetName(name string)
	GetPrevChildIndex() int
	GetNextChildIndex() int
	ChildIndex() int
	GetStorageClsid() ClassID
	SetStorageClsid(cid ClassID)
	GetStartBlock() int
	UseSmallBlocks() bool
	SetIndex(index int)
	WriteData(w io.Writer) error
	PreWrite()
	SetChildProperty(child int)
	GetIndex() int
}

type Property struct {
	name                                             string
	name_size                                        *ShortField
	property_type, node_color                        *ByteField
	previous_property, next_property, child_property *IntegerField
	storage_clsid                                    ClassID
	user_flags, seconds_1, days_1, seconds_2, days_2 *IntegerField
	start_block, size                                *IntegerField
	raw_data                                         []byte
	index                                            int
	next_child, prev_child                           Child
}

type DocumentProperty struct {
	*Property
	document *POIFSDocument
}

type DirectoryProperty struct {
	*Property
	children       []IProperty //map[string]IProperty
	children_names map[string]bool
}

type RootProperty struct {
	*DirectoryProperty
}

type blockProperty struct {
	*Property
}

type PropertyTable struct {
	header_block *HeaderBlock
	properties   []IProperty
	bigBlockSize POIFSBigBlockSize
	blocks       []BlockWritable
}

func CreateProperty() *Property {
	ret := &Property{}
	ret.raw_data = make([]byte, PROPERTY_SIZE)
	ret.name_size, _ = NewShortFieldWithOffset(_name_size_offset)
	ret.property_type, _ = NewByteFieldWithOffset(PROPERTY_TYPE_OFFSET)
	ret.node_color, _ = NewByteFieldWithOffset(_node_color_offset)
	ret.previous_property, _ = NewIntegerField(_previous_property_offset, _NO_INDEX, ret.raw_data)
	ret.next_property, _ = NewIntegerField(_next_property_offset, _NO_INDEX, ret.raw_data)
	ret.child_property, _ = NewIntegerField(_child_property_offset, _NO_INDEX, ret.raw_data)
	ret.storage_clsid = ClassIDFromBytes(ret.raw_data, _storage_clsid_offset)
	ret.user_flags, _ = NewIntegerField(_user_flags_offset, 0, ret.raw_data)
	ret.seconds_1, _ = NewIntegerField(_seconds_1_offset, 0, ret.raw_data)
	ret.days_1, _ = NewIntegerField(_days_1_offset, 0, ret.raw_data)
	ret.seconds_2, _ = NewIntegerField(_seconds_2_offset, 0, ret.raw_data)
	ret.days_2, _ = NewIntegerField(_days_2_offset, 0, ret.raw_data)
	ret.start_block, _ = NewIntegerFieldWithOffset(_start_block_offset)
	ret.size, _ = NewIntegerField(_size_offset, 0, ret.raw_data)
	ret.index = _NO_INDEX
	ret.SetName("")
	return ret
}

func NewProperty(index int, array []byte, offset int) *Property {
	ret := &Property{}
	ret.raw_data = make([]byte, PROPERTY_SIZE)
	copy(ret.raw_data[0:PROPERTY_SIZE], array[offset:])
	ret.name_size, _ = NewShortFieldFromBytes(_name_size_offset, ret.raw_data)
	ret.property_type, _ = NewByteFieldFromBytes(PROPERTY_TYPE_OFFSET, ret.raw_data)
	ret.node_color, _ = NewByteFieldFromBytes(_node_color_offset, ret.raw_data)
	ret.previous_property, _ = NewIntegerFieldFromBytes(_previous_property_offset, ret.raw_data)
	ret.next_property, _ = NewIntegerFieldFromBytes(_next_property_offset, ret.raw_data)
	ret.child_property, _ = NewIntegerFieldFromBytes(_child_property_offset, ret.raw_data)
	ret.storage_clsid = ClassIDFromBytes(ret.raw_data, _storage_clsid_offset)

	ret.user_flags, _ = NewIntegerField(_user_flags_offset, 0, ret.raw_data)
	ret.seconds_1, _ = NewIntegerFieldFromBytes(_seconds_1_offset, ret.raw_data)
	ret.days_1, _ = NewIntegerFieldFromBytes(_days_1_offset, ret.raw_data)
	ret.seconds_2, _ = NewIntegerFieldFromBytes(_seconds_2_offset, ret.raw_data)
	ret.days_2, _ = NewIntegerFieldFromBytes(_days_2_offset, ret.raw_data)
	ret.start_block, _ = NewIntegerFieldFromBytes(_start_block_offset, ret.raw_data)
	ret.size, _ = NewIntegerFieldFromBytes(_size_offset, ret.raw_data)
	ret.index = index

	name_length := int(ret.name_size.value/SHORT_SIZE) - 1
	if name_length < 1 {
		ret.name = ""
	} else {
		ret.name = ""
		char_array := make([]uint16, name_length)
		name_offset := 0
		for j := 0; j < name_length; j++ {
			char_array[j] = uint16(getShort(ret.raw_data, name_offset))
			name_offset += SHORT_SIZE
		}
		sLen := 0
		/*
			if !unicode.IsPrint(rune(char_array[0])) {
				sLen = 1
			}
		*/
		ret.name = string(utf16.Decode(char_array[sLen:]))
	}
	return ret
}

func isSmall(length int) bool {
	return length < _big_block_minimum_bytes
}

func (p *Property) UseSmallBlocks() bool {
	return isSmall(p.size.value)
}

func (p *Property) ShortDescription() string {
	return fmt.Sprintf("Property: \"%s\"", p.name)
}

/*
func (p *Property) SetNext(child Child) {
	p.next_child = child
	var index int
	if child == nil {
		index = _NO_INDEX
	} else {
		index = (child.(*Property)).index
	}
	p.next_property.Set(index, p.raw_data)
}
*/

func (p *Property) GetNextChild() Child {
	return p.next_child
}

func (p *Property) GetPreviousChild() Child {
	return p.prev_child
}

/*
func (p *Property) SetPrevious(child Child) {
	p.prev_child = child
	var index int
	if child == nil {
		index = _NO_INDEX
	} else {
		index = (child.(*Property)).index
	}
	p.previous_property.Set(index, p.raw_data)
}
*/

func (p *Property) GetSize() int {
	return p.size.value
}

func (p *Property) SetSize(size int) {
	p.size.Set(size, p.raw_data)
}

func (p *Property) SetNodeColor(nodeColor byte) {
	p.node_color.Set(nodeColor, p.raw_data)
}

func (p *Property) SetPropertyType(propertyType byte) {
	p.property_type.Set(propertyType, p.raw_data)
}

func (p *Property) SetStartBlock(startBlock int) {
	p.start_block.Set(startBlock, p.raw_data)
}

func (p *Property) IsDirectory() bool {
	panic("Shouldn't be reaching here")
}

func (p *Property) ChildIndex() int {
	return p.child_property.value
}

func (p *Property) GetIndex() int {
	return p.index
}

//child the child property's index in the Property Table
func (p *Property) SetChildProperty(child int) {
	p.child_property.Set(child, p.raw_data)
}

func (p *Property) GetPrevChildIndex() int {
	return p.previous_property.value
}

func (p *Property) GetNextChildIndex() int {
	return p.next_property.value
}

func (p *Property) GetName() string {
	return p.name
}

func (p *Property) SetName(name string) {
	limit := min(len(name), _max_name_length)

	p.name = name[0:limit]
	offset, j := 0, 0

	for j < limit {
		NewShortField(offset, int16(name[j]), p.raw_data)
		offset += SHORT_SIZE
		j += 1
	}

	for j < _max_name_length {
		NewShortField(offset, 0, p.raw_data)
		offset += SHORT_SIZE
		j += 1
	}

	p.name_size.Set(int16(limit+1)*SHORT_SIZE, p.raw_data)
}

func (p *Property) GetStorageClsid() ClassID {
	return p.storage_clsid
}

func (p *Property) SetStorageClsid(cid ClassID) {
	p.storage_clsid = cid
	if &cid == nil {
		for i := _storage_clsid_offset; i < _storage_clsid_offset+16; i++ {
			p.raw_data[i] = byte(0x00)
		}
	} else {
		cid.Write(p.raw_data, _storage_clsid_offset)
	}
}

func (p *Property) GetStartBlock() int {
	return p.start_block.value
}

func (p *Property) SetIndex(index int) {
	p.index = index
}

func (p *Property) WriteData(w io.Writer) error {
	//fmt.Printf("Writing property: %s\n[%v]\n\n", p.GetName(),
	//	strings.ToUpper(hex.EncodeToString(p.raw_data)))
	_, err := w.Write(p.raw_data)
	return err
}

func (p *Property) SetPreviousChild(child Child) {
	p.prev_child = child
	if child == nil {
		p.previous_property.Set(_NO_INDEX, p.raw_data)
	} else {
		p.previous_property.Set(child.(IProperty).GetIndex(), p.raw_data)
	}
}

func (p *Property) SetNextChild(child Child) {
	p.next_child = child
	if child == nil {
		p.next_property.Set(_NO_INDEX, p.raw_data)
	} else {
		p.next_property.Set(child.(IProperty).GetIndex(), p.raw_data)
	}
}

func NewDocProperty(name string, size int) *DocumentProperty {
	doc := &DocumentProperty{}
	doc.Property = CreateProperty()
	doc.SetName(name)
	doc.SetSize(size)
	doc.SetNodeColor(_NODE_BLACK)
	doc.SetPropertyType(DOCUMENT_TYPE)
	return doc
}

func NewDocPropertyFromData(index int, array []byte, offset int) *DocumentProperty {
	doc := &DocumentProperty{}
	doc.Property = NewProperty(index, array, offset)
	return doc

}

func (doc *DocumentProperty) ShouldUseSmallBlocks() bool {
	return doc.ShouldUseSmallBlocks()
}

func (doc *DocumentProperty) IsDirectory() bool {
	return false
}

func (doc *DocumentProperty) SetDocument(d *POIFSDocument) {
	if d == nil {
		fmt.Printf("Null Document passed to set for property: %v", *doc)
	}
	doc.document = d
}

func (doc *DocumentProperty) PreWrite() {
	//Do nothing
}

func NewDirProperty(name string) *DirectoryProperty {
	dir := &DirectoryProperty{}
	dir.Property = CreateProperty()
	dir.children = make([]IProperty, 0) //make(map[string]IProperty, 0)
	dir.children_names = make(map[string]bool)
	dir.SetName(name)
	dir.SetSize(0)
	dir.SetPropertyType(DIRECTORY_TYPE)
	dir.SetStartBlock(0)
	dir.SetNodeColor(_NODE_BLACK)
	return dir
}

func NewDirPropertyFromData(index int, array []byte, offset int) *DirectoryProperty {
	return &DirectoryProperty{Property: NewProperty(index, array, offset),
		children:       make([]IProperty, 0),
		children_names: make(map[string]bool),
	}
}

func (doc *DirectoryProperty) IsDirectory() bool {
	return true
}

func (doc *DirectoryProperty) ChangeName(prop IProperty, newName string) bool {
	oldName := prop.GetName()
	prop.SetName(newName)
	cleanNewName := prop.GetName()

	_, ok := doc.children_names[cleanNewName]

	if ok {
		//revert the change
		prop.SetName(oldName)
		return false
	} else {
		doc.children_names[cleanNewName] = true
		delete(doc.children_names, oldName)
		return true
	}
}

func (doc *DirectoryProperty) removeEntry(prop IProperty) bool {
	found := false
	var idx int
	var p IProperty
	for idx, p = range doc.children {
		if p == prop {
			found = true
			break
		}
	}

	if found {
		//Delete Entry
		doc.children = append(doc.children[:idx], doc.children[idx+1:]...)
		//Avoid potential memory leak, by nil out the pointer values
		doc.children[len(doc.children)-1] = nil
		doc.children = doc.children[:len(doc.children)-1]
	}
	return found
}

func (doc *DirectoryProperty) DelChild(prop IProperty) bool {
	if doc.removeEntry(prop) {
		delete(doc.children_names, prop.GetName())
		return true
	}
	return false
}

func (doc *DirectoryProperty) AddChild(prop IProperty) error {
	if _, ok := doc.children_names[prop.GetName()]; ok {
		return errors.New("Duplicate name \"" + prop.GetName() + "\"")
	}
	doc.children_names[prop.GetName()] = true
	doc.children = append(doc.children, prop)
	return nil
}

func (doc *DirectoryProperty) GetChildren() []IProperty {
	return doc.children
}

func (doc *DirectoryProperty) PreWrite() {
	if len(doc.children) <= 0 {
		return
	}
	PropertySorter(doc.compare).Sort(doc.children)
	midpoint := len(doc.children) / 2

	doc.SetChildProperty(doc.children[midpoint].GetIndex())
	doc.children[0].SetPreviousChild(nil)
	doc.children[0].SetNextChild(nil)
	for j := 1; j < midpoint; j++ {
		doc.children[j].SetPreviousChild(doc.children[j-1].(Child))
		doc.children[j].SetNextChild(nil)
	}
	if midpoint != 0 {
		doc.children[midpoint].SetPreviousChild(doc.children[midpoint-1].(Child))
	}
	if midpoint != len(doc.children)-1 {
		doc.children[midpoint].SetNextChild(doc.children[midpoint+1].(Child))
		for j := midpoint + 1; j < len(doc.children)-1; j++ {
			doc.children[j].SetPreviousChild(nil)
			doc.children[j].SetNextChild(doc.children[j+1].(Child))
		}
		doc.children[len(doc.children)-1].SetPreviousChild(nil)
		doc.children[len(doc.children)-1].SetNextChild(nil)
	} else {
		doc.children[midpoint].SetNextChild(nil)
	}
}

func (doc *DirectoryProperty) compare(p1, p2 IProperty) int {
	VBA_PROJECT := "_VBA_PROJECT"
	name1 := p1.GetName()
	name2 := p2.GetName()
	result := len(name1) - len(name2)
	if result == 0 {
		// _VBA_PROJECT, it seems, will always come last
		if strings.EqualFold(name1, VBA_PROJECT) {
			result = 1
		} else if strings.EqualFold(name2, VBA_PROJECT) {
			result = -1
		} else {
			if strings.HasPrefix(name1, "__") && strings.HasPrefix(name2, "__") {
				// Betweeen __SRP_0 and __SRP_1 just sort as normal
				if name1 < name2 {
					result = -1
				} else if name1 > name2 {
					result = 1
				} else {
					result = 0
				}
			} else if strings.HasPrefix(name1, "__") {
				// If only name1 is __XXX then this will be placed after name2
				result = 1
			} else if strings.HasPrefix(name2, "__") {
				// If only name2 is __XXX then this will be placed after name1
				result = -1
			} else {
				// The default case is to sort names ignoring case
				if name1 < name2 {
					result = -1
				} else if name1 > name2 {
					result = 1
				} else {
					result = 0
				}
			}
		}
	}
	return result
}

func NewRootProperty() *RootProperty {
	r := &RootProperty{}
	r.DirectoryProperty = NewDirProperty("Root Entry")
	r.SetNodeColor(_NODE_BLACK)
	r.SetPropertyType(ROOT_TYPE)
	r.SetStartBlock(END_OF_CHAIN)
	return r
}

func NewRootPropertyFromData(index int, array []byte, offset int) *RootProperty {
	r := &RootProperty{}
	r.DirectoryProperty = NewDirPropertyFromData(index, array, offset)
	r.name = "Root Entry"
	return r
}

func (p *RootProperty) SetSize(size int) {
	p.DirectoryProperty.SetSize(size * _block_size)
}

func newBlockProperty() *blockProperty {
	r := &blockProperty{}
	r.Property = CreateProperty()
	return r
}

func (bp *blockProperty) PreWrite() {
	//Do nothing
}

func (bp *blockProperty) IsDirectory() bool {
	return false
}

func convertToProperties(blocks []ListManagedBlock) ([]IProperty, error) {
	props := make([]IProperty, 0)
	for _, block := range blocks {
		data, err := block.Data()
		if err != nil {
			return nil, err
		}

		prop_count := len(data) / PROPERTY_SIZE
		offset := 0

		for k := 0; k < prop_count; k++ {
			switch data[offset+PROPERTY_TYPE_OFFSET] {
			case DIRECTORY_TYPE:
				props = append(props, NewDirPropertyFromData(len(props), data, offset))
			case DOCUMENT_TYPE:
				props = append(props, NewDocPropertyFromData(len(props), data, offset))
			case ROOT_TYPE:
				props = append(props, NewRootPropertyFromData(len(props), data, offset))
			default:
				props = append(props, nil)
			}
			offset += PROPERTY_SIZE
		}
	}

	return props, nil
}

func NewPropertyTable(hb *HeaderBlock) *PropertyTable {
	pt := &PropertyTable{}
	pt.header_block = hb
	pt.properties = make([]IProperty, 0)
	pt.AddProperty(NewRootProperty())
	pt.bigBlockSize = hb.bigBlockSize
	return pt
}

func NewPropertyTableWithProps(hb *HeaderBlock, props []IProperty) *PropertyTable {
	pt := &PropertyTable{}
	pt.header_block = hb
	pt.properties = props
	pt.bigBlockSize = hb.bigBlockSize
	pt.populatePropTree((pt.properties[0]).(ParentProperty)) //.(*DirectoryProperty))
	return pt
}

func NewPropertyTableWithList(hb *HeaderBlock, blocklist *rawDataBlockList) (*PropertyTable, error) {
	blocks, err := blocklist.Fetch(hb.GetPropertyStart(), -1)
	if err != nil {
		return nil, err
	}
	plist, err := convertToProperties(blocks)
	if err != nil {
		return nil, err
	}
	pt := NewPropertyTableWithProps(hb, plist)

	return pt, nil
}

//Prepare to be written
func (pt *PropertyTable) PreWrite() {
	// give each property its index
	for index, prop := range pt.properties {
		prop.SetIndex(index)
	}

	//alllocate the blocks for the property table

	pt.blocks = NewPropertyBlockArray(pt.bigBlockSize, pt.properties)

	// prepare each property for writing
	for _, prop := range pt.properties {
		prop.PreWrite()
	}
}

func (pt *PropertyTable) AddProperty(prop IProperty) {
	pt.properties = append(pt.properties, prop)
}

func (pt *PropertyTable) RemoveProperty(prop IProperty) {
	found := false
	var idx int
	var p IProperty
	for idx, p = range pt.properties {
		if p == prop {
			found = true
			break
		}
	}

	if found {
		//Delete Entry
		pt.properties = append(pt.properties[:idx], pt.properties[idx+1:]...)
		//Avoid potential memory leak, by nil out the pointer values
		pt.properties[len(pt.properties)-1] = nil
		pt.properties = pt.properties[:len(pt.properties)-1]
	}
}

func (pt *PropertyTable) GetRoot() ParentProperty {
	//its always the first element
	return (pt.properties[0]).(*RootProperty)
}

func (pt *PropertyTable) populatePropTree(root ParentProperty) { //(root *DirectoryProperty) {
	index := root.ChildIndex()
	if index == _NO_INDEX {
		return
	}

	var children Stack
	children.Push(pt.properties[index])
	for !children.IsEmpty() {
		p, err := children.Pop()
		if err != nil {
			break
		}
		prop, ok := p.(IProperty)
		if !ok {
			continue
		}
		root.AddChild(prop.(IProperty))
		if prop.IsDirectory() {
			pt.populatePropTree(prop.(*DirectoryProperty))
		}
		index = prop.GetPrevChildIndex()
		if index != _NO_INDEX {
			children.Push(pt.properties[index])
		}
		index = prop.GetNextChildIndex()
		if index != _NO_INDEX {
			children.Push(pt.properties[index])
		}
	}
}

func (pt *PropertyTable) GetStartBlock() int {
	return pt.header_block.GetPropertyStart()
}

func (pt *PropertyTable) SetStartBlock(index int) {
	pt.header_block.SetPropertyStart(index)
}

func (pt *PropertyTable) CountBlocks() int {
	return len(pt.blocks)
}

func (pt *PropertyTable) WriteBlocks(w io.Writer) error {
	for _, block := range pt.blocks {
		if err := block.WriteBlocks(w); err != nil {
			return err
		}
	}
	return nil
}
