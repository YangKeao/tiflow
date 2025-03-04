// Copyright 2023 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package simple

import (
	_ "embed"
	"encoding/json"

	"github.com/linkedin/goavro/v2"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/sink/codec/common"
)

//go:embed message.json
var avroSchemaBytes []byte

type marshaller interface {
	// MarshalCheckpoint marshals the checkpoint ts into bytes.
	MarshalCheckpoint(ts uint64) ([]byte, error)

	// MarshalDDLEvent marshals the DDL event into bytes.
	MarshalDDLEvent(event *model.DDLEvent) ([]byte, error)

	// MarshalRowChangedEvent marshals the row changed event into bytes.
	MarshalRowChangedEvent(event *model.RowChangedEvent, config *common.Config,
		handleKeyOnly bool, claimCheckFileName string) ([]byte, error)

	// Unmarshal the bytes into the given value.
	Unmarshal(data []byte, v any) error
}

func newMarshaller(format common.EncodingFormatType) (marshaller, error) {
	var (
		result marshaller
		err    error
	)
	switch format {
	case common.EncodingFormatJSON:
		result = newJSONMarshaller()
	case common.EncodingFormatAvro:
		result, err = newAvroMarshaller(string(avroSchemaBytes))
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.New("unknown encoding format")
	}
	return result, nil
}

type jsonMarshaller struct{}

func newJSONMarshaller() *jsonMarshaller {
	return &jsonMarshaller{}
}

// MarshalCheckpoint implement the marshaller interface
func (m *jsonMarshaller) MarshalCheckpoint(ts uint64) ([]byte, error) {
	msg := newResolvedMessage(ts)
	result, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}
	return result, nil
}

// MarshalDDLEvent implement the marshaller interface
func (m *jsonMarshaller) MarshalDDLEvent(event *model.DDLEvent) ([]byte, error) {
	var (
		msg *message
		err error
	)
	if event.IsBootstrap {
		msg, err = newBootstrapMessage(event.TableInfo)
	} else {
		msg, err = newDDLMessage(event)
	}
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}
	return value, nil
}

// MarshalRowChangedEvent implement the marshaller interface
func (m *jsonMarshaller) MarshalRowChangedEvent(
	event *model.RowChangedEvent, config *common.Config,
	handleKeyOnly bool, claimCheckFileName string,
) ([]byte, error) {
	msg, err := newDMLMessage(event, config, handleKeyOnly, claimCheckFileName)
	if err != nil {
		return nil, err
	}

	value, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}
	return value, nil
}

// Unmarshal implement the marshaller interface
func (m *jsonMarshaller) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

type avroMarshaller struct {
	codec *goavro.Codec
}

func newAvroMarshaller(schema string) (*avroMarshaller, error) {
	codec, err := goavro.NewCodec(schema)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &avroMarshaller{
		codec: codec,
	}, nil
}

// Marshal implement the marshaller interface
func (m *avroMarshaller) Marshal(v any) ([]byte, error) {
	return m.codec.BinaryFromNative(nil, v)
}

// MarshalCheckpoint implement the marshaller interface
func (m *avroMarshaller) MarshalCheckpoint(ts uint64) ([]byte, error) {
	msg := newResolvedMessageMap(ts)
	result, err := m.codec.BinaryFromNative(nil, msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}
	return result, nil
}

// MarshalDDLEvent implement the marshaller interface
func (m *avroMarshaller) MarshalDDLEvent(event *model.DDLEvent) ([]byte, error) {
	var msg map[string]interface{}
	if event.IsBootstrap {
		msg = newBootstrapMessageMap(event.TableInfo)
	} else {
		msg = newDDLMessageMap(event)
	}
	value, err := m.codec.BinaryFromNative(nil, msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}
	return value, nil
}

// MarshalRowChangedEvent implement the marshaller interface
func (m *avroMarshaller) MarshalRowChangedEvent(
	event *model.RowChangedEvent, config *common.Config,
	handleKeyOnly bool, claimCheckFileName string,
) ([]byte, error) {
	msg, err := newDMLMessageMap(event, config, handleKeyOnly, claimCheckFileName)
	if err != nil {
		return nil, err
	}
	value, err := m.codec.BinaryFromNative(nil, msg)
	if err != nil {
		return nil, errors.WrapError(errors.ErrEncodeFailed, err)
	}

	recycleMap(msg)
	return value, nil
}

// Unmarshal implement the marshaller interface
func (m *avroMarshaller) Unmarshal(data []byte, v any) error {
	native, _, err := m.codec.NativeFromBinary(data)
	if err != nil {
		return errors.Trace(err)
	}
	err = newMessageFromAvroNative(native, v.(*message))
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
