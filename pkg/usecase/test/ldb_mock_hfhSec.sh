#!/bin/bash

# Simple mock for the ldb binary supporting both 'select' and 'dump' commands
# Save as 'ldb' and make executable with chmod +x

read input

# Check if input matches the select pattern: select from KB/TABLE key HEX
if [[ $input =~ select[[:space:]]+from[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+key[[:space:]]+([[:xdigit:]]+)[[:space:]]+csv ]]; then
   key="${BASH_REMATCH[2]}"
   # If the key matches, return the predefined result
   if [[ "$key" == "6f4789ffa01b315e" ]]; then
          cat << 'EOF'
6f4789ffa01b315e,243d1a05246f0d12,
6f4789ffa01b315e,253d1a01246f0d1a,
EOF
       exit 0
   fi

# Check if input matches the dump pattern: dump KB/TABLE hex -1 sector HEX
elif [[ $input =~ dump[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+hex[[:space:]]+-1[[:space:]]+sector[[:space:]]+([[:xdigit:]]+) ]]; then
   kb_table="${BASH_REMATCH[1]}"
   sector="${BASH_REMATCH[2]}"
   
   # If the kb/table and sector match the expected values
   if [[ "$kb_table" == "test_kb/hfhSec" && "$sector" == "80" ]]; then
       # Return predefined dump result set
       cat << 'EOF'
8000008f8ec6b880,80e8e93a8806537c,
80000353f68b7ffa,9da0a33fe6c8a597,
8000035fef22e1da,4692116d8f6b010d,
8000036eacf054df,7e25e0132aec36db,
800006419acf96e0,8863bd4c8601b7ee,
800007879aa0d5ea,6f337347f4e7119c,
800007be5dcef6b5,729d6c15a4b1663c,
80000910f9a29ad2,7132dd07b4eb26a8,
80000910f9a29ad2,7132dd07b4eb26ac,
8000099ae9cec507,9cf85e3f66fd85cb,
800bcde023d32dab,a2cf6995494cedea,
800bcdf3216a5075,87714e3ee2a79795,
800bce19fa04d754,5e325c0fb66b33fa,
800bcef84050ae49,717bbddc0e0137ff,
800bcf0d68a675e1,5d85d939067d0fbb,
800bcf8b9098e661,84d8743ebae9af2b,
800bcf94ab0a2bc9,93aa9f7eb64d8fb8,
800bcfb521d78a28,59763a970b18ad26,
800bcfbb6547f737,8a2b757e9fe837eb,
800bcfcba97a9e60,5872350c8601b7ab,
800bcfee30643892,8b63fd2ca56937f4,
800bcff416b8d04a,9ca0355ee8afa39e,
800bd0199802f889,9de27d65eaaf189f,
800bd024f6dd4236,95a2db3fa649b3b0,
800bd0384177a09e,7caaf83d160993b4,
800bd047d14c63bf,8932bd0cb6abb7ae,
800bd05e5cd91785,884cc45870a3fdb6,
800bd05e5cd91785,984cc458f0a3fdb6,
800bd05e5cd91785,994cc458f2a3fdb6,
800bd0b1ebbf2f60,935c9c3c6befb7bd,
800bd0f12d44bb2a,8b819d34e360db39,
800bd14a34dee285,6bdc349b087cad4e,
80fffb3f1b9d45ae,853fec1428ea85cf,
80fffc274799361b,77db651eedb9072f,
80fffc5a823d6c5f,aa74fd78ebcf8b8d,
80fffc9445e60022,a588f528a6dd61bb,
80fffd0b529366ae,81639c4eb66907e7,
80fffd732f4f2f6f,8a63bd5c8601b7ee,
80fffe3c533c715d,8263bd4c8601b7bb,
80fffe600a161ffe,485c353b203cad4a,
80ffff20104c2781,829981c317379eca,
80ffff3f6d7c6677,772b727ea76748b8,
EOF
       exit 0
   fi
fi

exit 0