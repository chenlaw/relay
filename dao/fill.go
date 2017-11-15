/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package dao

import (
	"github.com/Loopring/ringminer/chainclient"
	"github.com/Loopring/ringminer/types"
)

type Fill struct {
	ID            int    `gorm:"column:id;primary_key;"`
	RingHash      string `gorm:"column:ring_hash;varchar(82)"`
	PreOrderHash  string `gorm:"column:pre_order_hash;varchar(82)"`
	NextOrderHash string `gorm:"column:next_order_hash;varchar(82)"`
	OrderHash     string `gorm:"column:order_hash;type:varchar(82)"`
	AmountS       []byte `gorm:"column:amount_s;type:varchar(30)"`
	AmountB       []byte `gorm:"column:amount_b;type:varchar(30)"`
	LrcReward     []byte `gorm:"column:lrc_reward;type:varchar(30)"`
	LrcFee        []byte `gorm:"column:lrc_fee;type:varchar(30)"`
}

// convert chainclient/orderFilledEvent to dao/fill
func (f *Fill) ConvertDown(src *chainclient.OrderFilledEvent) error {
	var err error

	if f.AmountS, err = src.AmountS.MarshalText(); err != nil {
		return err
	}
	if f.AmountB, err = src.AmountB.MarshalText(); err != nil {
		return err
	}
	if f.LrcReward, err = src.LrcReward.MarshalText(); err != nil {
		return err
	}
	if f.LrcFee, err = src.LrcFee.MarshalText(); err != nil {
		return err
	}

	f.RingHash = types.BytesToHash(src.Ringhash).Hex()
	f.PreOrderHash = types.BytesToHash(src.PreOrderHash).Hex()
	f.NextOrderHash = types.BytesToHash(src.NextOrderHash).Hex()
	f.OrderHash = types.Bytes2Hex(src.OrderHash)

	return nil
}
