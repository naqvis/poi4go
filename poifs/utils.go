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
	"encoding/hex"
	"errors"
	"strings"
)

type FixedField interface {
	ReadFromBytes(data []byte)
	WriteToBytes(data []byte)
	String() string
}

type IntegerField struct {
	value, offset int
}

type ShortField struct {
	value  int16
	offset int
}

type ByteField struct {
	value  byte
	offset int
}

type ClassID struct {
	bytes []byte
}

func NewIntegerFieldWithOffset(offset int) (*IntegerField, error) {
	if offset < 0 {
		return nil, errors.New("negative offset")
	}
	return &IntegerField{offset: offset}, nil
}

func NewIntegerFieldWithVal(offset, value int) (*IntegerField, error) {
	f, err := NewIntegerFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.value = value
	return f, nil
}

func NewIntegerFieldFromBytes(offset int, data []byte) (*IntegerField, error) {
	f, err := NewIntegerFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.ReadFromBytes(data)
	return f, nil
}

func NewIntegerField(offset, value int, data []byte) (*IntegerField, error) {
	f, err := NewIntegerFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.Set(value, data)
	return f, nil
}

func (f *IntegerField) ReadFromBytes(data []byte) {
	f.value = getInt(data, f.offset)
}

func (f *IntegerField) Set(val int, data []byte) {
	f.value = val
	f.WriteToBytes(data)
}

func (f *IntegerField) WriteToBytes(data []byte) {
	putInt(data, f.offset, f.value)
}

func (f *IntegerField) String() string {
	return string(f.value)
}

func NewShortFieldWithOffset(offset int) (*ShortField, error) {
	if offset < 0 {
		return nil, errors.New("negative offset")
	}
	return &ShortField{offset: offset}, nil
}

func NewShortFieldWithVal(offset int, value int16) (*ShortField, error) {
	f, err := NewShortFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.value = value
	return f, nil
}

func NewShortFieldFromBytes(offset int, data []byte) (*ShortField, error) {
	f, err := NewShortFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.ReadFromBytes(data)
	return f, nil
}

func NewShortField(offset int, value int16, data []byte) (*ShortField, error) {
	f, err := NewShortFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.Set(value, data)
	return f, nil
}

func (f *ShortField) ReadFromBytes(data []byte) {
	f.value = getShort(data, f.offset)
}

func (f *ShortField) Set(val int16, data []byte) {
	f.value = val
	f.WriteToBytes(data)
}

func (f *ShortField) WriteToBytes(data []byte) {
	putShort(data, f.offset, f.value)
}

func (f *ShortField) String() string {
	return string(f.value)
}

func NewByteFieldWithOffset(offset int) (*ByteField, error) {
	if offset < 0 {
		return nil, errors.New("negative offset")
	}
	return &ByteField{offset: offset}, nil
}

func NewByteFieldWithVal(offset int, value byte) (*ByteField, error) {
	f, err := NewByteFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.value = value
	return f, nil
}

func NewByteFieldFromBytes(offset int, data []byte) (*ByteField, error) {
	f, err := NewByteFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.ReadFromBytes(data)
	return f, nil
}

func NewByteField(offset int, value byte, data []byte) (*ByteField, error) {
	f, err := NewByteFieldWithOffset(offset)
	if err != nil {
		return nil, err
	}
	f.Set(value, data)
	return f, nil
}

func (f *ByteField) ReadFromBytes(data []byte) {
	f.value = data[f.offset]
}

func (f *ByteField) Set(val byte, data []byte) {
	f.value = val
	f.WriteToBytes(data)
}

func (f *ByteField) WriteToBytes(data []byte) {
	data[f.offset] = f.value
}

func (f *ByteField) String() string {
	return string(f.value)
}

func ClassIDFromBytes(src []byte, offset int) ClassID {
	cid := new(ClassID)
	cid.Read(src, offset)
	return *cid
}

func NewClassID() ClassID {
	cid := ClassID{}
	cid.bytes = make([]byte, 16)
	return cid
}

func ClassIDFromString(str string) (ClassID, error) {
	res := ClassID{}
	if len(str) < 38 {
		return res, errors.New("Invalid GUID. Length should be 38 chars")
	}
	cleanStr := strings.Trim(str, "{}")
	parts := strings.Split(cleanStr, "-")
	if len(parts) != 5 {
		return res, errors.New("Invalid GUID. GUID should have five '-' separators")
	}
	var err error
	res.bytes, err = hex.DecodeString(strings.Join(parts, ""))
	if err != nil {
		return res, err
	}
	return res, nil
}

func (cid *ClassID) SetBytes(bytes []byte) {
	copy(cid.bytes[0:], bytes[0:])
}

func (cid *ClassID) Read(src []byte, offset int) []byte {
	cid.bytes = make([]byte, 16)

	//Read double word
	cid.bytes[0] = src[3+offset]
	cid.bytes[1] = src[2+offset]
	cid.bytes[2] = src[1+offset]
	cid.bytes[3] = src[0+offset]

	//Read first word
	cid.bytes[4] = src[5+offset]
	cid.bytes[5] = src[4+offset]

	//Read second word
	cid.bytes[6] = src[7+offset]
	cid.bytes[7] = src[6+offset]

	//Read 8 bytes

	for i := 8; i < 16; i++ {
		cid.bytes[i] = src[i+offset]
	}

	return cid.bytes
}

func (cid *ClassID) Write(dst []byte, offset int) error {
	if len(dst) < 16 {
		return errors.New("Destinate byte[] must have room for at least 16 bytes.")
	}

	//Write double word
	dst[0+offset] = cid.bytes[3]
	dst[1+offset] = cid.bytes[2]
	dst[2+offset] = cid.bytes[1]
	dst[3+offset] = cid.bytes[0]

	//Write first word
	dst[4+offset] = cid.bytes[5]
	dst[5+offset] = cid.bytes[5]

	//Write second word
	dst[6+offset] = cid.bytes[7]
	dst[7+offset] = cid.bytes[6]

	//Write 8 bytes
	for i := 8; i < 16; i++ {
		dst[i+offset] = cid.bytes[i]
	}
	return nil
}

func (cid *ClassID) String() string {
	return strings.ToUpper("{" +
		hex.EncodeToString(cid.bytes[:4]) + "-" +
		hex.EncodeToString(cid.bytes[4:6]) + "-" +
		hex.EncodeToString(cid.bytes[6:8]) + "-" +
		hex.EncodeToString(cid.bytes[8:10]) + "-" +
		hex.EncodeToString(cid.bytes[10:16]) + "}")
}
