// Code generated by protoc-gen-go. DO NOT EDIT.
// source: test.proto

package testdata

import proto "google.golang.org/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Ping struct {
	Content              string   `protobuf:"bytes,1,opt,name=Content" json:"Content,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Ping) Reset()         { *m = Ping{} }
func (m *Ping) String() string { return proto.CompactTextString(m) }
func (*Ping) ProtoMessage()    {}
func (*Ping) Descriptor() ([]byte, []int) {
	return fileDescriptor_test_ab5a0806b9eb7ae2, []int{0}
}
func (m *Ping) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Ping.Unmarshal(m, b)
}
func (m *Ping) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Ping.Marshal(b, m, deterministic)
}
func (dst *Ping) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Ping.Merge(dst, src)
}
func (m *Ping) XXX_Size() int {
	return xxx_messageInfo_Ping.Size(m)
}
func (m *Ping) XXX_DiscardUnknown() {
	xxx_messageInfo_Ping.DiscardUnknown(m)
}

var xxx_messageInfo_Ping proto.InternalMessageInfo

func (m *Ping) GetContent() string {
	if m != nil {
		return m.Content
	}
	return ""
}

type Pong struct {
	Content              string   `protobuf:"bytes,2,opt,name=Content" json:"Content,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Pong) Reset()         { *m = Pong{} }
func (m *Pong) String() string { return proto.CompactTextString(m) }
func (*Pong) ProtoMessage()    {}
func (*Pong) Descriptor() ([]byte, []int) {
	return fileDescriptor_test_ab5a0806b9eb7ae2, []int{1}
}
func (m *Pong) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Pong.Unmarshal(m, b)
}
func (m *Pong) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Pong.Marshal(b, m, deterministic)
}
func (dst *Pong) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Pong.Merge(dst, src)
}
func (m *Pong) XXX_Size() int {
	return xxx_messageInfo_Pong.Size(m)
}
func (m *Pong) XXX_DiscardUnknown() {
	xxx_messageInfo_Pong.DiscardUnknown(m)
}

var xxx_messageInfo_Pong proto.InternalMessageInfo

func (m *Pong) GetContent() string {
	if m != nil {
		return m.Content
	}
	return ""
}

func init() {
	proto.RegisterType((*Ping)(nil), "testdata.Ping")
	proto.RegisterType((*Pong)(nil), "testdata.Pong")
}

func init() { proto.RegisterFile("test.proto", fileDescriptor_test_ab5a0806b9eb7ae2) }

var fileDescriptor_test_ab5a0806b9eb7ae2 = []byte{
	// 85 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2a, 0x49, 0x2d, 0x2e,
	0xd1, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x00, 0xb1, 0x53, 0x12, 0x4b, 0x12, 0x95, 0x14,
	0xb8, 0x58, 0x02, 0x32, 0xf3, 0xd2, 0x85, 0x24, 0xb8, 0xd8, 0x9d, 0xf3, 0xf3, 0x4a, 0x52, 0xf3,
	0x4a, 0x24, 0x18, 0x15, 0x18, 0x35, 0x38, 0x83, 0x60, 0x5c, 0xb0, 0x8a, 0x7c, 0x54, 0x15, 0x4c,
	0x28, 0x2a, 0x92, 0xd8, 0xc0, 0x86, 0x1a, 0x03, 0x02, 0x00, 0x00, 0xff, 0xff, 0x01, 0xcd, 0x56,
	0xda, 0x62, 0x00, 0x00, 0x00,
}
