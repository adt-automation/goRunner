
function add(a, b, c) {
    //all goRunner input arguments to javascript are strings
    // var key = {}
    // key.n = "c0209c4274d580abc0429292b88673142008f02121bfb6df19ad34c8b6e052d05c6afa43261a871a4c84b9d073e8111fde2d60b7597569c692309dc1fa61032d144683f3b003ecb437c0dd0eba708d2260b645033f6be11cd78d729f596b26decd1018d4484d1315d66b1db7390bd40f3a050fc542b827593e7c69666c91c069"
    // key.e = "10001"
    // return rsa.encrypt("toto", key)
    return parseFloat(a) + parseFloat(b) + parseFloat(c); //so cast to float before adding
}

function sub(a, b) {
    //all goRunner input arguments to javascript are strings
    return parseFloat(a) - parseFloat(b); //so cast to float before subtracting
}