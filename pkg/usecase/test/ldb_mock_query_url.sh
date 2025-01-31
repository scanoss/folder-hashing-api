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
fi

# If input doesn't match or key is not the expected one, exit with error
exit 1

