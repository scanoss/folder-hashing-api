#!/bin/bash
###
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Copyright (C) 2018-2023 SCANOSS.COM
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
###

#!/bin/bash

# Simple mock for the ldb binary
read input

# Check if input matches the expected pattern: select from KB/TABLE key HEX
if [[ $input =~ key[[:space:]]+([[:xdigit:]]+)[[:space:]]+csv ]]; then
   key="${BASH_REMATCH[1]}"
   # If the key matches, return the predefined result
   if [[ "$key" == "0134f476e2548425" ]]; then
       echo "0134f476e2548425,d07adbb449a3c005bb82bd238469f25d7afe2bf761f35720fbb5f5b9089985f8bf8d05fd461aeaf0a7030c90e295236bca2b0c01cfd717cadcfef8af9cd4f61dfd8146a7f8fe65c32e6107c8ef2daaacbe5c127f4b20627047dadc8979f8b6d23178339785db602545c6384cf430585e82c07104bc95e8f98d0e7a16ebbd389625774e7f613d6563455772e717dabc282f3e51d18b3d1c4fc4cc349df4f4ca86c1327cb7595ef6ee8201aec06b346cc2d4911dafb317020dfd06b84755982d99"
       exit 0
   fi
   if [[ "$key" == "fd1dc18e1e1364bb" ]]; then
       echo "fd1dc18e1e1364bb,7ac02cabd26dbeb2e795003fa376acb4ce12b3f48a6fd3c2cf2c856be07277e52f6f95a459d321b6ddd740d11a5dd500789bb72eed9f83be9b8df6179c8e72a3f0148cf250aded3eff86e5c16b56b4e1574b2cb22665959f2d623abe48dc06e29a73d660b5ef612819c59017903d2317e01d7d3a973a256c56affbbe27b44a93fd24f982f6618192208ff3b300648da552896e0080075d13"       
       exit 0
   fi
   if [[ "$key" == "f0491d197565a8e6" ]]; then
       echo "f0491d197565a8e6,df659af45d70dcfd950c2c2277a7cf1a4ffb3c28ae4c42ad9bfbd88859fadea985a8ce93cda8932c1b313e0dea939c4410e25fc2a960bcb5b5de703b97247cd18e09a4a0d5f6001ba4ba0b19613e3d525877202c0bcfb3eb2713a75c9d38b311b781e2ac4c4caebaa9db13644c9bd9abcfab17cc94966ff6e0b17c62abb6c6342bf3a6399d48a243c76d8551"
       exit 0
   fi
      if [[ "$key" == "0b44e04d55a30938" ]]; then
       echo "0b44e04d55a30938,4e014afd4cfd613673d985bb4e04756548da81a0e6c506917f002f4c82e8aa7c77592b663da37e8dd13dfa6dc42834f7b4c9039e21b47ea518bb35cdaff2deb3cef7b65c007fb0b27106da9779afcee5c5937eab0903b6a44e83e528e78a8395353f2043957b4df757ec874c5f6d6909bb3a00df2e19e75d91c32efadc287932f63f9b19d4b66cde67a5b4f64f4b476bdb03d57aca925fed52c0b3fc26ac61288c581167707153e604af0ba4444e112e3e9bc85e6fc2c875973910c1c4c1b674ad6416"
       exit 0
   fi
   fi
   if [[ "$key" == "4bb4f66d0d9a24c7" ]]; then
      echo "4bb4f66d0d9a24c7,8bcda24aadbbfd70d34dbbb323e93721e2a89a7cb3116d33fec1acb93a7a5ec6bc58308c5f5f4789fb9bcaa62c97e65834a67238ff3c3cba7aa1a99f779461b1a251afd01718c48d61d0b75f01a715dabd200b914ca5befccc746ec8220fe127e78c385939ca37a797c6a51607330b9353ecbc3340fbc807e84d1b98669562272025a34e4238e10a2d7f5cf27526c8a1b57aa633857ca9d896139bd2643546a5e7bf19fe3fa093069802ae2a6ae5a576fa8a42e5fb20483085ed7372b0bb9b8e9349"
      exit 0
   fi
   if [[ "$key" == "7d03ec9913c31d83" ]]; then
      echo "7d03ec9913c31d83,e2a89db35a813a6f034fe9e0d25fffa168e6895292f0a4771edac9d9b79d3d075b744073d8aef82b600989cc01ac145004d4ae70b4584110e1dfd46515220f26396efaec06c71d01e138c20a85c827da8eab540a885e53c1bf21e6879ac8aa99b2f8a6a6796c500418b7a92368baa857d97ca943d6e8d3450705295ece27676904b383f0bbfdd23a7eb778acbedb965db230b5"
      exit 0
   fi
   if [[ "$key" == "002cf90d0427a119" ]]; then
      echo "002cf90d0427a119,2c4884a96e467388877ed54e85afc1d70bbc828d7ae10a528fc9e167622c6a87d951914f3ca32b3b896eda8c081f3e95c8702fbffa2e6e69f4091fc8b4dadc4962909c1d5acd2b062c160d3ec5119714c742ba3db4bd5b4d082d42f824c2efa76b7a318e3ec314e3c4a8042b3c3cec9071d000861eff902c71408c62449cf2b9b602eb98c67f6fda94ea3532bae59ab5e6fcf3b6efe197116c32c8"
      exit 0
   fi
   
fi

# If input doesn't match or key is not the expected one, exit with error
exit 1

