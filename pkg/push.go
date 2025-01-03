// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package pitaya

import (
	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/constants"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/protos"
	"github.com/echo766/pitaya/pkg/util"
)

func (app *App) SendPushToUser(route string, v interface{}, uid string, frontId string, frontType string) error {
	data, err := util.SerializeOrRaw(app.serializer, v)
	if err != nil {
		return err
	}

	if frontId == "" {
		return constants.ErrFrontendTypeNotSpecified
	}

	push := &protos.Push{
		Route: route,
		Uid:   uid,
		Data:  data,
	}

	return app.rpcClient.SendPush(uid, &cluster.Server{ID: frontId, Type: frontType}, push)
}

func (app *App) SendPushBytesToUsers(route string, data []byte, uids []string, frontendType string) ([]string, error) {
	if !app.server.Frontend && frontendType == "" {
		return uids, constants.ErrFrontendTypeNotSpecified
	}

	var notPushedUids []string

	for _, uid := range uids {
		if app.server.Type == frontendType {
			if s := app.sessionPool.GetSessionByUID(uid); s != nil {
				if err := s.Push(route, data); err != nil {
					notPushedUids = append(notPushedUids, uid)
					logger.Log.Errorf("Session push message error, ID=%d, UID=%s, Error=%s",
						s.ID(), s.UID(), err.Error())
				}
			} else {
				notPushedUids = append(notPushedUids, uid)
				logger.Log.Warnf("Session not found, UID=%s", uid)
			}
		} else if app.rpcClient != nil {
			push := &protos.Push{
				Route: route,
				Uid:   uid,
				Data:  data,
			}
			if err := app.rpcClient.SendPush(uid, &cluster.Server{Type: frontendType}, push); err != nil {
				notPushedUids = append(notPushedUids, uid)
				logger.Log.Errorf("RPCClient send message error, UID=%s, SvType=%s, Error=%s", uid, frontendType, err.Error())
			}
		} else {
			notPushedUids = append(notPushedUids, uid)
		}
	}

	if len(notPushedUids) != 0 {
		return notPushedUids, constants.ErrPushingToUsers
	}

	return nil, nil
}

// SendPushToUsers sends a message to the given list of users
func (app *App) SendPushToUsers(route string, v interface{}, uids []string, frontendType string) ([]string, error) {
	data, err := util.SerializeOrRaw(app.serializer, v)
	if err != nil {
		return uids, err
	}

	logger.Log.Debugf("Type=PushToUsers Route=%s, Data=%+v, SvType=%s, #Users=%d", route, v, frontendType, len(uids))

	return app.SendPushBytesToUsers(route, data, uids, frontendType)
}
