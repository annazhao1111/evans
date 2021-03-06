package di

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/ktr0731/evans/adapter/grpc"
	"github.com/ktr0731/evans/adapter/inputter"
	"github.com/ktr0731/evans/adapter/presenter"
	"github.com/ktr0731/evans/adapter/protobuf"
	"github.com/ktr0731/evans/config"
	"github.com/ktr0731/evans/entity"
	environment "github.com/ktr0731/evans/entity/env"
	"github.com/ktr0731/evans/usecase/port"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
)

var (
	env     environment.Environment
	envOnce sync.Once
)

func initEnv(cfg *config.Config) (rerr error) {
	envOnce.Do(func() {
		paths, err := resolveProtoPaths(cfg)
		if err != nil {
			rerr = err
			return
		}

		files := resolveProtoFiles(cfg)
		desc, err := protobuf.ParseFile(files, paths)
		if err != nil {
			rerr = err
			return
		}

		gRPCClient, err := GRPCClient(cfg)
		if err != nil {
			rerr = err
			return
		}

		headers := make([]entity.Header, 0, len(cfg.Request.Header))
		for _, h := range cfg.Request.Header {
			headers = append(headers, entity.Header{Key: h.Key, Val: h.Val})
		}
		if gRPCClient.ReflectionEnabled() {
			var svcs []entity.Service
			var msgs []entity.Message
			svcs, msgs, err = gRPCClient.ListServices()
			if err != nil {
				rerr = errors.Wrap(err, "failed to list services by gRPC reflection")
				return
			}
			env = environment.NewFromServices(svcs, msgs, headers)
		} else {
			env = environment.New(desc, headers)

			if pkg := cfg.Default.Package; pkg != "" {
				if err := env.UsePackage(pkg); err != nil {
					rerr = errors.Wrapf(err, "failed to set package to env as a default package: %s", pkg)
					return
				}
			}
		}

		if svc := cfg.Default.Service; svc != "" {
			if err := env.UseService(svc); err != nil {
				rerr = errors.Wrapf(err, "failed to set service to env as a default service: %s", svc)
				return
			}
		}
	})
	return
}

func Env(cfg *config.Config) (environment.Environment, error) {
	if err := initEnv(cfg); err != nil {
		return nil, err
	}
	return env, nil
}

func resolveProtoPaths(cfg *config.Config) ([]string, error) {
	paths := make([]string, 0, len(cfg.Default.ProtoPath))
	encountered := map[string]bool{}
	parser := shellwords.NewParser()
	parser.ParseEnv = true

	parse := func(p string) (string, error) {
		res, err := parser.Parse(p)
		if err != nil {
			return "", err
		}
		if len(res) > 1 {
			return "", errors.New("failed to parse proto path")
		}
		// empty path
		if len(res) == 0 {
			return "", nil
		}
		return res[0], nil
	}

	for _, p := range cfg.Default.ProtoPath {
		path, err := parse(p)
		if err != nil {
			return nil, err
		}

		if encountered[path] || path == "" {
			continue
		}
		encountered[path] = true
		paths = append(paths, path)
	}

	return paths, nil
}

func resolveProtoFiles(conf *config.Config) []string {
	files := make([]string, 0, len(conf.Default.ProtoFile))
	for _, f := range conf.Default.ProtoFile {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

var (
	jsonCLIPresenter     *presenter.JSONPresenter
	jsonCLIPresenterOnce sync.Once
)

func initJSONCLIPresenter() error {
	jsonCLIPresenterOnce.Do(func() {
		jsonCLIPresenter = presenter.NewJSONWithIndent()
	})
	return nil
}

var (
	jsonFileInputter     *inputter.JSONFile
	jsonFileInputterOnce sync.Once
)

func initJSONFileInputter(in io.Reader) error {
	jsonFileInputterOnce.Do(func() {
		jsonFileInputter = inputter.NewJSONFile(in)
	})
	return nil
}

var (
	promptInputter     *inputter.PromptInputter
	promptInputterOnce sync.Once
)

func initPromptInputter(cfg *config.Config) (err error) {
	promptInputterOnce.Do(func() {
		var e environment.Environment
		e, err = Env(cfg)
		promptInputter = inputter.NewPrompt(cfg.Input.PromptFormat, e)
	})
	return
}

var (
	gRPCClient     entity.GRPCClient
	gRPCClientOnce sync.Once
)

func initGRPCClient(cfg *config.Config) error {
	var err error
	gRPCClientOnce.Do(func() {
		addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
		if cfg.Request.Web {
			var b port.DynamicBuilder
			b, err = DynamicBuilder()
			if err != nil {
				return
			}
			gRPCClient = grpc.NewWebClient(addr, cfg.Server.Reflection, b)
		} else {
			gRPCClient, err = grpc.NewClient(addr, cfg.Server.Reflection)
		}
	})
	return err
}

func GRPCClient(cfg *config.Config) (entity.GRPCClient, error) {
	if err := initGRPCClient(cfg); err != nil {
		return nil, err
	}
	return gRPCClient, nil
}

var (
	dynamicBuilder     *protobuf.DynamicBuilder
	dynamicBuilderOnce sync.Once
)

func initDynamicBuilder() error {
	dynamicBuilderOnce.Do(func() {
		dynamicBuilder = protobuf.NewDynamicBuilder()
	})
	return nil
}

func DynamicBuilder() (port.DynamicBuilder, error) {
	if err := initDynamicBuilder(); err != nil {
		return nil, err
	}
	return dynamicBuilder, nil
}

type initializer struct {
	f []func() error

	resultCache error
	done        bool
}

func (i *initializer) register(f ...func() error) {
	i.f = append(i.f, f...)
}

func (i *initializer) init() error {
	if i.done {
		return i.resultCache
	}

	i.done = true

	var result error
	for _, f := range i.f {
		if err := f(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

var (
	initer     *initializer
	initerOnce sync.Once
)

func initDependencies(cfg *config.Config, in io.Reader) error {
	initerOnce.Do(func() {
		initer = &initializer{}
		initer.register(
			func() error { return initJSONFileInputter(in) },
			func() error { return initPromptInputter(cfg) },
			func() error { return initGRPCClient(cfg) },
			func() error { return initEnv(cfg) },
			initJSONCLIPresenter,
			initDynamicBuilder,
		)
	})
	if err := initer.init(); err != nil {
		gRPCClient.Close(context.Background())
		return err
	}
	return nil
}
