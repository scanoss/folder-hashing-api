#!/bin/bash

# Simple mock for the ldb binary supporting both 'select' and 'dump' commands
# Save as 'ldb' and make executable with chmod +x

read input

# Check if input matches the select pattern: select from KB/TABLE key HEX
if [[ $input =~ select[[:space:]]+from[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+key[[:space:]]+([[:xdigit:]]+)[[:space:]]+csv ]]; then
   key="${BASH_REMATCH[2]}"
   # If the key matches, return the predefined result
   if [[ "$key" == "8172bd3ef0ab37b4" ]]; then
       echo "8172bd3ef0ab37b4,f98fc3f728a8b4d4,fd1dc18e1e1364bb,"
       echo "8172bd3ef0ab37b4,862e8c6492b29efd,f0491d197565a8e6,"
       exit 0
   fi
      if [[ "$key" == "5f3dcd05ab272b52" ]]; then
       echo "5f3dcd05ab272b52,643a57d3b6f27064,0b44e04d55a30938,"
       echo "5f3dcd05ab272b52,dcbad65736bef466,e05e988faaf26f25,"
       echo "5f3dcd05ab272b52,dcbad6d736bef564,10a1de5e02e61411,"
       echo "5f3dcd05ab272b52,dcbad6d736bef564,3c130c43938a5447,"
       exit 0
   fi
      if [[ "$key" == "243d1a05246f0d12" ]]; then
       echo "243d1a05246f0d12,1aa976edf5044c53,4bb4f66d0d9a24c7,"
       exit 0
   fi
    if [[ "$key" == "253d1a01246f0d1a" ]]; then
       echo "253d1a01246f0d1a,dab778ca7f764c03,7d03ec9913c31d83,"
       exit 0
   fi
       if [[ "$key" == "8b819d34e360db39" ]]; then
       echo "8b819d34e360db39,d20c7a2bf044affb,002cf90d0427a119,"
       exit 0
   fi


# Check if input matches the dump pattern: dump KB/TABLE hex -1 sector HEX
elif [[ $input =~ dump[[:space:]]+([[:alnum:]_]+/[[:alnum:]_]+)[[:space:]]+hex[[:space:]]+-1[[:space:]]+sector[[:space:]]+([[:xdigit:]]+) ]]; then
   kb_table="${BASH_REMATCH[1]}"
   sector="${BASH_REMATCH[2]}"
   
   # If the kb/table and sector match the expected values
   if [[ "$kb_table" == "test_kb/hfh" && "$sector" == "81" ]]; then
       # Return predefined dump result set
       cat << 'EOF'
81e4b058637f034a,f7ddf528f7ff76df,00671fab4a895b36,
81e4b922c326870a,d051489eff9c047c,0024dc92ae1f6bd2,
81e4bd188a29b73e,bee9acb05806654a,014716a2a543731a,
81e4d01e4c7aec88,7439739f8d30d5fd,01d4b5e9e374aeea,
81e4d01e4c7aec88,d2316e1da748e2cc,00c7104943d0aa6e,
81e4d91d6b4d475c,79ed83d4b6129c24,00794fd3b832f8fb,
81e4e4158a2a813e,76fa6aeff72e36f1,0160d2ca8666835e,
81e4e51d45446e6c,7cedef2453ecbe7e,0096d020bdd4a316,
81e4f01e483f6c88,c5005ef805732243,00ac6aebe96782bf,
81e4fd112c1937eb,8c365bc7d6bb566c,0101771f4153ea61,
81e5099056bf47b7,f57b7870eacabe1f,01d5f39e8334c7e2,
81e5354c8609bfab,b18d84f55a9d4ef2,015b6361f24dfd67,
81e5354c8609bfab,b9cfdca12bf8b5f4,00fe95c0376dbe3f,
81e5490c55f213d0,e93e30220fbd8a61,009d067b2c150a19,
81e549ff1dcf568e,c2bbbf947df3927b,00e857c928c0516f,
81e55034a90ff319,8b19870a4ffa97ce,00cc73f7dc5af1e8,
81e55034a90ff319,8f19851bcfdbd36f,006f79421a7608f0,
81e55d13949128b6,eedd911fcd3a1ce2,01869310e0db0c22,
81e579bc476d33be,ebe815ecee73ecff,014a7cef1174f28a,
81e579bc476d33be,ee9a8cf12aefd6df,00cdd9cbc00d075d,
81e5841034e337c0,ff24be7bef74d6bf,013ae455b5a1b5d0,
81e58428329114c4,6ae641612ef48035,009a8339b7be9584,
81e58499a9135e65,c29be4a87ff5e3cb,00e09af4cce4583f,
81e59629d48e5873,bebea6966246e830,004ea20f66dc1a95,
81e5a1086ac44b32,57bccfce8fd91253,0110ec0402a110ba,
81e5a13976a5419f,b0c9d291feeeadaf,000d48fe13a94890,
81e5ad68260dba39,db2d9a65b7cc9a1e,003bca7042c68493,
81e5b3780df13bbf,3edcd5787d9c068c,00de85c9ecbd3392,
81e5c5da6019e61e,582eacf60fa3b838,01b46fd7563d4fa8,
81e5cd7ad46c2223,90f064a40366f3b2,014242e0558a56dd,
81e5d765013832d0,925cc28ee1b4d84f,0178597914d9b45b,
81e5dd69564af341,05ee2b2233c28b1e,007a061e6fa204fc,
81e5df5e0a75973e,6b8775dafea67f91,017befde848a5f67,
81e5e94f119f32bc,7167e365dd6e83ec,007fdf82b3b873ab,
81e5ec3e8d999f12,5e6514af5f7e86da,01b721e266523606,
81e5ed77dd5d6009,72f76a887db9c3e5,00f0a6d076779aa8,
81e6037b64d77ccf,bd9c390f09cda302,00e0873a7f9873be,
81e61128e66b37e2,d5fbb7d586edd3ee,012b3e1c79306942,
81e6195ed36720dc,3987870f7c91ef76,00100dccabb4951f,
81e61d0fb2196bda,959fea093b5b7abc,0195678d65d420c8,
81e61f18c8d9272f,bf8b2f49bc476bfc,01fc7852fd2e9a67,
81e622c3770eb49e,9cbd81f6c7cdd65b,01976c4d291c5d5b,
81e62c64851de425,ac6b7ffebb6effff,00580007f56e5f1c,
81e6307dcc42cd88,77b5dfa7fdb13079,0167267db8896bd4,
81e6315d4a9a8cac,39bb783c96dc8ab5,008aae677efe2a28,
8172bd3ef0ab37b4,f98fc3f728a8b4d5,fd1dc18e1e1364bb,
EOF
       exit 0
   fi
fi

# If input doesn't match any pattern or parameters are not the expected ones, exit with error
exit 0