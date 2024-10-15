package logger_test

import (
	"bytes"
	"encoding/json"
	"flag"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"go.uber.org/zap"
	"time"
)

var _ = Describe("Logger", func() {
	var (
		opts   *log.Options
		buffer *bytes.Buffer
		logger *zap.Logger
	)

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		opts = &log.Options{
			Output: buffer,
		}
	})

	When("creating a new logger", func() {
		Context("and only development mode is set to true", func() {
			BeforeEach(func() {
				opts.Development = true
				logger = log.New(log.UseOptions(opts))
			})

			It("should output debug log", func() {
				logger.Debug("debug message")
				Expect(buffer.String()).To(ContainSubstring("debug message"))
			})

			It("should output stacktrace at warn level", func() {
				logger.Warn("warn message")
				Expect(buffer.String()).To(ContainSubstring("logger_test.go"))
			})
		})

		Context("and only development mode is set to false", func() {
			BeforeEach(func() {
				opts.Development = false
				logger = log.New(log.UseOptions(opts))
			})

			It("should output info log", func() {
				logger.Info("info message")
				Expect(buffer.String()).To(ContainSubstring("info message"))
			})

			It("should not output debug log", func() {
				logger.Debug("debug message")
				Expect(buffer.String()).ToNot(ContainSubstring("debug message"))
			})

			It("should output stacktrace at error level", func() {
				logger.Error("error message")
				Expect(buffer.String()).To(ContainSubstring("logger_test.go"))
			})
		})
	})

	When("flags are bound", func() {
		var fs *flag.FlagSet

		BeforeEach(func() {
			fs = flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(buffer)
			opts.BindFlags(fs)
		})

		It("should use default options for missing flags", func() {
			args := []string{"-log-devel=false"}
			err := fs.Parse(args)
			opts.SetDefaults()

			Expect(err).ToNot(HaveOccurred())
			Expect(opts.Level.Enabled(zap.InfoLevel)).To(BeTrue())
			Expect(opts.StacktraceLevel.Enabled(zap.ErrorLevel)).To(BeTrue())
		})

		Context("and their arguments are valid", func() {
			It("should use provided mode development mode", func() {
				args := []string{"-log-devel=true"}
				err := fs.Parse(args)

				Expect(err).ToNot(HaveOccurred())
				Expect(opts.Development).To(BeTrue())
			})

			It("should set provided log level", func() {
				args := []string{"-log-level=debug"}
				err := fs.Parse(args)

				Expect(err).ToNot(HaveOccurred())
				Expect(opts.Level.Enabled(zap.DebugLevel)).To(BeTrue())
			})

			It("should set provided stacktrace level", func() {
				args := []string{"-log-stacktrace-level=panic"}
				err := fs.Parse(args)

				Expect(err).ToNot(HaveOccurred())
				Expect(opts.StacktraceLevel.Enabled(zap.PanicLevel)).To(BeTrue())
			})

			It("should set provided encoder", func() {
				args := []string{"-log-encoder=json"}
				err := fs.Parse(args)
				Expect(err).ToNot(HaveOccurred())

				logger = log.New(log.UseOptions(opts))
				logger.Info("info message")

				var res map[string]string
				Expect(json.Unmarshal(buffer.Bytes(), &res)).To(Succeed())
			})

			It("should set provided time encoder", func() {
				args := []string{"-log-time-encoder=rfc3339", "-log-encoder=json"}
				err := fs.Parse(args)
				Expect(err).ToNot(HaveOccurred())

				logger = log.New(log.UseOptions(opts))
				logger.Info("info message")

				var res map[string]string
				Expect(json.Unmarshal(buffer.Bytes(), &res)).To(Succeed())
				Expect(res).To(HaveKey("ts"))

				_, err = time.Parse(time.RFC3339, res["ts"])
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("and their arguments are invalid", func() {
			It("should return error for unknown log level", func() {
				args := []string{"-log-level=unknown"}
				err := fs.Parse(args)

				Expect(err).To(HaveOccurred())
			})

			It("should return error for unknown stacktrace level", func() {
				args := []string{"-log-stacktrace-level=unknown"}
				err := fs.Parse(args)

				Expect(err).To(HaveOccurred())
			})

			It("should return error for unknown encoder", func() {
				args := []string{"-log-encoder=unknown"}
				err := fs.Parse(args)

				Expect(err).To(HaveOccurred())
			})

			It("should return error for unknown time encoder", func() {
				args := []string{"-log-time-encoder=unknown"}
				err := fs.Parse(args)

				Expect(err).To(HaveOccurred())
			})
		})
	})
})
