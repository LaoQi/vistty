package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

type profileConfig struct {
	cpuProfile   string
	memProfile   string
	mutexProfile string
	traceFile    string
	fps          bool
	cpuStarted   bool
	traceStarted bool
}

func (p *profileConfig) start() error {
	if p.cpuProfile != "" {
		f, err := os.Create(p.cpuProfile)
		if err != nil {
			return fmt.Errorf("create cpu profile: %w", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return fmt.Errorf("start cpu profile: %w", err)
		}
		p.cpuStarted = true
	}

	if p.traceFile != "" {
		f, err := os.Create(p.traceFile)
		if err != nil {
			return fmt.Errorf("create trace file: %w", err)
		}
		if err := trace.Start(f); err != nil {
			f.Close()
			return fmt.Errorf("start trace: %w", err)
		}
		p.traceStarted = true
	}

	if p.mutexProfile != "" {
		runtime.SetMutexProfileFraction(1)
	}

	return nil
}

func (p *profileConfig) stop() {
	if p.cpuStarted {
		pprof.StopCPUProfile()
	}

	if p.memProfile != "" {
		f, err := os.Create(p.memProfile)
		if err == nil {
			pprof.WriteHeapProfile(f)
			f.Close()
		}
	}

	if p.mutexProfile != "" {
		f, err := os.Create(p.mutexProfile)
		if err == nil {
			pprof.Lookup("mutex").WriteTo(f, 0)
			f.Close()
		}
		runtime.SetMutexProfileFraction(0)
	}

	if p.traceStarted {
		trace.Stop()
	}
}
