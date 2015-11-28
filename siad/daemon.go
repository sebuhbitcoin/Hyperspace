package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/explorer"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/profile"

	"github.com/spf13/cobra"
)

// preprocessConfig checks the configuration values and performs cleanup on
// incorrect-but-allowed values.
func preprocessConfig() {
	// If the port numbers decode as an integer, prepend ":".
	_, err := strconv.Atoi(config.Siad.RPCaddr)
	if err == nil {
		config.Siad.RPCaddr = ":" + config.Siad.RPCaddr
	}
	_, err = strconv.Atoi(config.Siad.HostAddr)
	if err == nil {
		config.Siad.HostAddr = ":" + config.Siad.HostAddr
	}
}

// startDaemonCmd uses the config parameters to start siad.
func startDaemon() error {
	// Establish multithreading.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Print a startup message.
	//
	// TODO: This message can be removed once the api starts up in under 1/2
	// second.
	fmt.Println("Loading...")
	loadStart := time.Now()

	// Clean up the configuration input.
	preprocessConfig()

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
	if err != nil {
		return err
	}
	cs, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
	if err != nil {
		return err
	}
	var e *explorer.Explorer
	if config.Siad.Explorer {
		e, err = explorer.New(cs, filepath.Join(config.Siad.SiaDir, modules.ExplorerDir))
		if err != nil {
			return err
		}
	}
	tpool, err := transactionpool.New(cs, gateway)
	if err != nil {
		return err
	}
	wallet, err := wallet.New(cs, tpool, filepath.Join(config.Siad.SiaDir, modules.WalletDir))
	if err != nil {
		return err
	}
	miner, err := miner.New(cs, tpool, wallet, filepath.Join(config.Siad.SiaDir, modules.MinerDir))
	if err != nil {
		return err
	}
	host, err := host.New(cs, tpool, wallet, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, modules.HostDir))
	if err != nil {
		return err
	}
	renter, err := renter.New(cs, wallet, tpool, filepath.Join(config.Siad.SiaDir, modules.RenterDir))
	if err != nil {
		return err
	}
	srv, err := api.NewServer(
		config.Siad.APIaddr,
		config.Siad.RequiredUserAgent,
		config.Siad.LimitedAPI,
		cs,
		e,
		gateway,
		host,
		miner,
		renter,
		tpool,
		wallet,
	)
	if err != nil {
		return err
	}

	// Bootstrap to the network.
	if !config.Siad.NoBootstrap {
		// connect to 3 random bootstrap nodes
		perm := crypto.Perm(len(modules.BootstrapPeers))
		for _, i := range perm[:3] {
			go gateway.Connect(modules.BootstrapPeers[i])
		}
	}

	// Print a 'startup complete' message.
	//
	// TODO: This message can be removed once the api starts up in under 1/2
	// second.
	startupTime := time.Since(loadStart)
	fmt.Println("Finished loading in", startupTime.Seconds(), "seconds")

	// Start serving api requests.
	err = srv.Serve()
	if err != nil {
		return err
	}
	return nil
}

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(*cobra.Command, []string) {
	// Create the profiling directory if profiling is enabled.
	if config.Siad.Profile {
		go profile.StartContinuousProfile(config.Siad.ProfileDir)
	}

	// Start siad. startDaemon will only return when it is shutting down.
	err := startDaemon()
	if err != nil {
		fmt.Println(err)
	}
}
