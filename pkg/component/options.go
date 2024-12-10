// Copyright (c) nano Authors and TFG Co. All Rights Reserved.
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

package component

type (
	Options struct {
		Handler  bool
		Remote   bool
		Name     string              // component name
		NameFunc func(string) string // rename handler name
	}

	// Option used to customize handler
	Option func(options *Options)
)

// WithName used to rename component name
func WithName(name string) Option {
	return func(opt *Options) {
		opt.Name = name
	}
}

// WithNameFunc override handler name by specific function
// such as: strings.ToUpper/strings.ToLower
func WithNameFunc(fn func(string) string) Option {
	return func(opt *Options) {
		opt.NameFunc = fn
	}
}

// WithRemote used to mark component as remote
func WithRemote() Option {
	return func(opt *Options) {
		opt.Remote = true
	}
}

// WithHandler used to mark component as handler
func WithHandler() Option {
	return func(opt *Options) {
		opt.Handler = true
	}
}
