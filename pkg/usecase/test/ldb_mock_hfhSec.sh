#!/bin/bash

# Simple mock for the ldb binary supporting both 'select' and 'dump' commands
# Save as 'ldb' and make executable with chmod +x

read input

# Check if input matches the select pattern: select from KB/TABLE key HEX
if [[ $input =~ select[[:space:]]+from[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+key[[:space:]]+([[:xdigit:]]+)[[:space:]]+csv ]]; then
   key="${BASH_REMATCH[2]}"
   # If the key matches, return the predefined result
   if [[ "$key" == "00075de93df18ea6" ]]; then
       echo "00075de93df18ea6,271a83cbd2ce447978f5b2613706c2928758197999218c06921d82b96c09e04bd8734a0f3b56785a55fd79b53d10b8b369ac01a45083d17b3bf867db84b2696ce6a401106929958886d61a588dec8a9a02eb218a4d2f223b8049fcefc7c0819a89ead32e2406fb94af25809fa0fbd3c1c3571b742324d76e22c80ea3ca7d9d85e0586ab3d6845cf3676199bac77f8a94460601e2f81f1ddce57e67e04d22b3"
       exit 0
   fi

# Check if input matches the dump pattern: dump KB/TABLE hex -1 sector HEX
elif [[ $input =~ dump[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+hex[[:space:]]+-1[[:space:]]+sector[[:space:]]+([[:xdigit:]]+) ]]; then
   kb_table="${BASH_REMATCH[1]}"
   sector="${BASH_REMATCH[2]}"
   
   # If the kb/table and sector match the expected values
   if [[ "$kb_table" == "test_kb/hfhSec" && "$sector" == "6a" ]]; then
       # Return predefined dump result set
       cat << 'EOF'
6a00168a94ae6238,6b0a6c14734147e0,0134f476e2548425,
6a001837e5fb300a,af9686ce69caed3e,01e487bbfbd6d66d,
6a002eb8f3375f17,7faf6afb7fecf97f,0003d7cf64cacf2b,
6a003ced048a4180,eb3b364b68cb7c96,0060180c6dd159e2,
6a003ced058a4181,c47f36226ca32e11,009427157386242a,
6a00428eebb4120d,ddb61c34e3dd0c27,0067dddb8d7b5390,
6a004a86a6e7be30,ea0dadb6f7ffbefc,0021748ce14cda0d,
6a00502cd6a922dc,efce099ff7cf43dc,016d8956e916e6e1,
6a00584478a3d298,b219d77342458cf5,00b1f1e405ad7412,
6a005866b4fb12a0,18c5ca135c3e6825,0002cfa6a1507136,
EOF
       exit 0
   fi
fi

# If input doesn't match any pattern or parameters are not the expected ones, exit with error
exit 1