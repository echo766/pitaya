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
	"context"
	"time"

	"github.com/echo766/pitaya/pkg/cluster"
	"github.com/echo766/pitaya/pkg/component"
	"github.com/echo766/pitaya/pkg/config"
	"github.com/echo766/pitaya/pkg/interfaces"
	"github.com/echo766/pitaya/pkg/metrics"
	"github.com/echo766/pitaya/pkg/route"
	"github.com/echo766/pitaya/pkg/router"
	"github.com/echo766/pitaya/pkg/session"
	"github.com/echo766/pitaya/pkg/worker"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/proto"
)

var DefaultApp Pitaya

// Configure configures the app
func Configure(
	isFrontend bool,
	serverType string,
	serverMode ServerMode,
	serverMetadata map[string]string,
	cfgs ...*viper.Viper,
) {
	builder := NewBuilderWithConfigs(
		isFrontend,
		serverType,
		serverMode,
		serverMetadata,
		config.NewConfig(cfgs...),
	)
	DefaultApp = builder.Build()
	session.DefaultSessionPool = builder.SessionPool
}

func GetDieChan() chan bool {
	return DefaultApp.GetDieChan()
}

func SetDebug(debug bool) {
	DefaultApp.SetDebug(debug)
}

func SetHeartbeatTime(interval time.Duration) {
	DefaultApp.SetHeartbeatTime(interval)
}

func GetZoneID() string {
	return DefaultApp.GetZoneID()
}

// GetServerUniqueID returns the generated server id
func GetServerID() string {
	return DefaultApp.GetServerID()
}

func GetMetricsReporters() []metrics.Reporter {
	return DefaultApp.GetMetricsReporters()
}

func GetServer() *cluster.Server {
	return DefaultApp.GetServer()
}

func GetServerByID(id string) (*cluster.Server, error) {
	return DefaultApp.GetServerByID(id)
}

func GetServersByType(t string) (map[string]*cluster.Server, error) {
	return DefaultApp.GetServersByType(t)
}

func GetServers() []*cluster.Server {
	return DefaultApp.GetServers()
}

func GetSessionFromCtx(ctx context.Context) session.Session {
	return DefaultApp.GetSessionFromCtx(ctx)
}

// GetRouteFromCtx retrieves a session from a given context
func GetRouteFromCtx(ctx context.Context) *route.Route {
	return DefaultApp.GetRouteFromCtx(ctx)
}

func Start() {
	DefaultApp.Start()
}

func SetDictionary(dict map[string]uint16) error {
	return DefaultApp.SetDictionary(dict)
}

func AddRoute(serverType string, routingFunction router.RoutingFunc) error {
	return DefaultApp.AddRoute(serverType, routingFunction)
}

func Shutdown() {
	DefaultApp.Shutdown()
}

func StartWorker() {
	DefaultApp.StartWorker()
}

func RegisterRPCJob(rpcJob worker.RPCJob) error {
	return DefaultApp.RegisterRPCJob(rpcJob)
}

func IsRunning() bool {
	return DefaultApp.IsRunning()
}

func RPC(ctx context.Context, routeStr string, reply proto.Message, arg proto.Message) error {
	return DefaultApp.RPC(ctx, routeStr, reply, arg)
}

func RPCTo(ctx context.Context, serverID, routeStr string, reply proto.Message, arg proto.Message) error {
	return DefaultApp.RPCTo(ctx, serverID, routeStr, reply, arg)
}

func ReliableRPC(routeStr string, metadata map[string]interface{}, reply, arg proto.Message) (jid string, err error) {
	return DefaultApp.ReliableRPC(routeStr, metadata, reply, arg)
}

func ReliableRPCWithOptions(routeStr string, metadata map[string]interface{}, reply, arg proto.Message, opts *config.EnqueueOpts) (jid string, err error) {
	return DefaultApp.ReliableRPCWithOptions(routeStr, metadata, reply, arg, opts)
}

func SendPushToUsers(route string, v interface{}, uids []string, frontendType string) ([]string, error) {
	return DefaultApp.SendPushToUsers(route, v, uids, frontendType)
}

func SendPushToUser(route string, v interface{}, uid string, frontId string, frontendType string) error {
	return DefaultApp.SendPushToUser(route, v, uid, frontId, frontendType)
}

func SendKickToUsers(uids []string, frontendType string) ([]string, error) {
	return DefaultApp.SendKickToUsers(uids, frontendType)
}

func GroupCreate(ctx context.Context, groupName string) error {
	return DefaultApp.GroupCreate(ctx, groupName)
}

func GroupCreateWithTTL(ctx context.Context, groupName string, ttlTime time.Duration) error {
	return DefaultApp.GroupCreateWithTTL(ctx, groupName, ttlTime)
}

func GroupMembers(ctx context.Context, groupName string) ([]string, error) {
	return DefaultApp.GroupMembers(ctx, groupName)
}

func GroupBroadcast(ctx context.Context, frontendType, groupName, route string, v interface{}) error {
	return DefaultApp.GroupBroadcast(ctx, frontendType, groupName, route, v)
}

func GroupContainsMember(ctx context.Context, groupName, uid string) (bool, error) {
	return DefaultApp.GroupContainsMember(ctx, groupName, uid)
}

func GroupAddMember(ctx context.Context, groupName, uid string) error {
	return DefaultApp.GroupAddMember(ctx, groupName, uid)
}

func GroupRemoveMember(ctx context.Context, groupName, uid string) error {
	return DefaultApp.GroupRemoveMember(ctx, groupName, uid)
}

func GroupRemoveAll(ctx context.Context, groupName string) error {
	return DefaultApp.GroupRemoveAll(ctx, groupName)
}

func GroupCountMembers(ctx context.Context, groupName string) (int, error) {
	return DefaultApp.GroupCountMembers(ctx, groupName)
}

func GroupRenewTTL(ctx context.Context, groupName string) error {
	return DefaultApp.GroupRenewTTL(ctx, groupName)
}

func GroupDelete(ctx context.Context, groupName string) error {
	return DefaultApp.GroupDelete(ctx, groupName)
}

func Register(c component.Component, options ...component.Option) {
	DefaultApp.Register(c, options...)
}

func RegisterModule(module interfaces.Module, name string) error {
	return DefaultApp.RegisterModule(module, name)
}

func RegisterModuleAfter(module interfaces.Module, name string) error {
	return DefaultApp.RegisterModuleAfter(module, name)
}

func RegisterModuleBefore(module interfaces.Module, name string) error {
	return DefaultApp.RegisterModuleBefore(module, name)
}

func GetModule(name string) (interfaces.Module, error) {
	return DefaultApp.GetModule(name)
}

func AddSDListener(listener cluster.SDListener) {
	DefaultApp.AddSDListener(listener)
}