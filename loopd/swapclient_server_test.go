package loopd

import (
	"context"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop"
	"github.com/lightninglabs/loop/labels"
	"github.com/lightninglabs/loop/looprpc"
	mock_lnd "github.com/lightninglabs/loop/test"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

var (
	testnetAddr, _ = btcutil.NewAddressScriptHash(
		[]byte{123}, &chaincfg.TestNet3Params,
	)

	mainnetAddr, _ = btcutil.NewAddressScriptHash(
		[]byte{123}, &chaincfg.MainNetParams,
	)

	chanID1 = lnwire.NewShortChanIDFromInt(1)
	chanID2 = lnwire.NewShortChanIDFromInt(2)
	chanID3 = lnwire.NewShortChanIDFromInt(3)

	peer1 = route.Vertex{1}
	peer2 = route.Vertex{2}

	channel1 = lndclient.ChannelInfo{
		Active:        false,
		ChannelID:     chanID1.ToUint64(),
		PubKeyBytes:   peer1,
		LocalBalance:  10000,
		RemoteBalance: 0,
		Capacity:      10000,
	}

	channel2 = lndclient.ChannelInfo{
		Active:        true,
		ChannelID:     chanID2.ToUint64(),
		PubKeyBytes:   peer2,
		LocalBalance:  10000,
		RemoteBalance: 0,
		Capacity:      10000,
	}

	channel3 = lndclient.ChannelInfo{
		Active:        true,
		ChannelID:     chanID3.ToUint64(),
		PubKeyBytes:   peer2,
		LocalBalance:  10000,
		RemoteBalance: 0,
		Capacity:      10000,
	}
)

// TestValidateConfTarget tests all failure and success cases for our conf
// target validation function, including the case where we replace a zero
// target with the default provided.
func TestValidateConfTarget(t *testing.T) {
	const (
		// Various input confirmation values for tests.
		zeroConf int32 = 0
		oneConf  int32 = 1
		twoConf  int32 = 2
		fiveConf int32 = 5

		// defaultConf is the default confirmation target we use for
		// all tests.
		defaultConf = 6
	)

	tests := []struct {
		name           string
		confTarget     int32
		expectedTarget int32
		expectErr      bool
	}{
		{
			name:           "zero conf, get default",
			confTarget:     zeroConf,
			expectedTarget: defaultConf,
			expectErr:      false,
		},
		{
			name:       "one conf, get error",
			confTarget: oneConf,
			expectErr:  true,
		},
		{
			name:           "two conf, ok",
			confTarget:     twoConf,
			expectedTarget: twoConf,
			expectErr:      false,
		},
		{
			name:           "five conf, ok",
			confTarget:     fiveConf,
			expectedTarget: fiveConf,
			expectErr:      false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			target, err := validateConfTarget(
				test.confTarget, defaultConf,
			)

			haveErr := err != nil
			if haveErr != test.expectErr {
				t.Fatalf("expected err: %v, got: %v",
					test.expectErr, err)
			}

			if target != test.expectedTarget {
				t.Fatalf("expected: %v, got: %v",
					test.expectedTarget, target)
			}
		})
	}
}

// TestValidateLoopInRequest tests validation of loop in requests.
func TestValidateLoopInRequest(t *testing.T) {
	tests := []struct {
		name           string
		external       bool
		confTarget     int32
		expectErr      bool
		expectedTarget int32
	}{
		{
			name:           "external and htlc conf set",
			external:       true,
			confTarget:     1,
			expectErr:      true,
			expectedTarget: 0,
		},
		{
			name:           "external and no conf",
			external:       true,
			confTarget:     0,
			expectErr:      false,
			expectedTarget: 0,
		},
		{
			name:           "not external, zero conf",
			external:       false,
			confTarget:     0,
			expectErr:      false,
			expectedTarget: loop.DefaultHtlcConfTarget,
		},
		{
			name:           "not external, bad conf",
			external:       false,
			confTarget:     1,
			expectErr:      true,
			expectedTarget: 0,
		},
		{
			name:           "not external, ok conf",
			external:       false,
			confTarget:     5,
			expectErr:      false,
			expectedTarget: 5,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			external := test.external
			conf, err := validateLoopInRequest(
				test.confTarget, external,
			)

			haveErr := err != nil
			if haveErr != test.expectErr {
				t.Fatalf("expected err: %v, got: %v",
					test.expectErr, err)
			}

			if conf != test.expectedTarget {
				t.Fatalf("expected: %v, got: %v",
					test.expectedTarget, conf)
			}
		})
	}
}

// TestValidateLoopOutRequest tests validation of loop out requests.
func TestValidateLoopOutRequest(t *testing.T) {
	tests := []struct {
		name            string
		chain           chaincfg.Params
		confTarget      int32
		destAddr        btcutil.Address
		label           string
		channels        []lndclient.ChannelInfo
		outgoingChanSet []uint64
		amount          int64
		maxRoutingFee   int64
		err             error
		expectedTarget  int32
	}{
		{
			name:           "mainnet address with mainnet backend",
			chain:          chaincfg.MainNetParams,
			destAddr:       mainnetAddr,
			label:          "label ok",
			confTarget:     2,
			err:            nil,
			expectedTarget: 2,
		},
		{
			name:           "mainnet address with testnet backend",
			chain:          chaincfg.TestNet3Params,
			destAddr:       mainnetAddr,
			label:          "label ok",
			confTarget:     2,
			err:            errIncorrectChain,
			expectedTarget: 0,
		},
		{
			name:           "testnet address with testnet backend",
			chain:          chaincfg.TestNet3Params,
			destAddr:       testnetAddr,
			label:          "label ok",
			confTarget:     2,
			err:            nil,
			expectedTarget: 2,
		},
		{
			name:           "testnet address with mainnet backend",
			chain:          chaincfg.MainNetParams,
			destAddr:       testnetAddr,
			label:          "label ok",
			confTarget:     2,
			err:            errIncorrectChain,
			expectedTarget: 0,
		},
		{
			name:           "invalid label",
			chain:          chaincfg.MainNetParams,
			destAddr:       mainnetAddr,
			label:          labels.Reserved,
			confTarget:     2,
			err:            labels.ErrReservedPrefix,
			expectedTarget: 0,
		},
		{
			name:           "invalid conf target",
			chain:          chaincfg.MainNetParams,
			destAddr:       mainnetAddr,
			label:          "label ok",
			confTarget:     1,
			err:            errConfTargetTooLow,
			expectedTarget: 0,
		},
		{
			name:           "default conf target",
			chain:          chaincfg.MainNetParams,
			destAddr:       mainnetAddr,
			label:          "label ok",
			confTarget:     0,
			err:            nil,
			expectedTarget: 9,
		},
		{
			name:       "valid amount for default channel set",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel1, channel2, channel3,
			},
			amount:         20000,
			err:            nil,
			expectedTarget: 2,
		},
		{
			name: "invalid amount for default channel " +
				"set",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel1, channel2, channel3,
			},
			amount:         25000,
			err:            errBalanceTooLow,
			expectedTarget: 0,
		},
		{
			name: "inactive channel in outgoing channel " +
				"set",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel1, channel2, channel3,
			},
			outgoingChanSet: []uint64{
				chanID1.ToUint64(),
			},
			amount:         1000,
			err:            errBalanceTooLow,
			expectedTarget: 0,
		},
		{
			name:       "outgoing channel set balance is enough",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel1, channel2, channel3,
			},
			outgoingChanSet: []uint64{
				chanID2.ToUint64(),
			},
			amount:         1000,
			err:            nil,
			expectedTarget: 2,
		},
		{
			name: "outgoing channel set balance not " +
				"sufficient",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel1, channel2, channel3,
			},
			outgoingChanSet: []uint64{
				chanID2.ToUint64(),
			},
			amount:         20000,
			err:            errBalanceTooLow,
			expectedTarget: 0,
		},
		{
			name:       "amount with routing fee too high",
			chain:      chaincfg.MainNetParams,
			destAddr:   mainnetAddr,
			label:      "label ok",
			confTarget: 2,
			channels: []lndclient.ChannelInfo{
				channel2,
			},
			amount:         10000,
			maxRoutingFee:  100,
			err:            errBalanceTooLow,
			expectedTarget: 0,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			lnd := mock_lnd.NewMockLnd()
			lnd.Channels = test.channels

			req := &looprpc.LoopOutRequest{
				Amt:               test.amount,
				MaxSwapRoutingFee: test.maxRoutingFee,
				OutgoingChanSet:   test.outgoingChanSet,
				Label:             test.label,
				SweepConfTarget:   test.confTarget,
			}

			conf, err := validateLoopOutRequest(
				ctx, lnd.Client, &test.chain, req,
				test.destAddr,
			)
			require.True(t, errors.Is(err, test.err))
			require.Equal(t, test.expectedTarget, conf)
		})
	}
}
