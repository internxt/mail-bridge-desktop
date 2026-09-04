package crypto

import (
	"bytes"
	"testing"
)

// These vectors were produced by the JS libraries this package must
// interoperate with, @noble/post-quantum and @noble/ciphers, from a
// deterministic seed of 0x00..0x1f:
//
//	const keys = hybrid.keygen(seed);
//	const { cipherText, sharedSecret } = hybrid.encapsulate(keys.publicKey, ...);
//	const wrapped = aeskw(sharedSecret).encrypt(sessionKey);
//
// They are the only real check on this port. A Go-only round trip would pass
// even if the seed expansion or the combiner were wrong, because encryption
// and decryption would be wrong in the same way; matching noble's output byte
// for byte is what proves the bridge can read mail the web client wrote.
const (
	vectorSeed = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

	vectorSharedSecret = "5e70189092bcfba07156827149e3c4bc630716656df7474da88b23a1c1b0974e"
	vectorSessionKey   = "030a11181f262d343b424950575e656c737a81888f969da4abb2b9c0c7ced5dc"
	vectorWrappedKey   = "bd63602009fbdfa089fddc18c873e55bd687069c96fd1646f08fd2b4b1410b8e183095c78afa6acc"

	vectorCiphertext = "6e15bf536f9f1c4348d5091021f6a4b0d72be0a826e455bc95da87bb68dae12ab622d31316c4b0b4d76127adc0b5d825b117da37a9029ece81a8a613da297efeaad6b3782a8e444bd24ce8eb3ef2bbbb03adb602cb2be8a6937354731f584c6b46c710ef54e93d5630278432aa4859970d15325583f56a1f6fa31231957c04281e6e96683c0fa0de19be09189476bc629258b614fe45b065d36a82b5389d2d977809bed90dbc83b35f09a04c5e2799245a920e9783a5a2cab7ba9770577a2c7394cd7f126283c9ad1053d644b7fdd139ebbe9bbf6c943e09f2d11ab66e3a716dbcb484b44626ae4f5bb4edc89dbfe19ac8c4d1bebb20f148180c030cccdfc78c59b20615766c4b7daa7779c8b7ce88bc6eab596de582972add524b44f9c449c082fd38380d9df4156280d516d0f6fc2a08873fcd258c6ec24219e67d3b9d945a022aadc9aea1e1490caa067008208d2637a26403ae9dd0a4e48849ea2c96bcf3071400866b26b675a7163ce162bad108f4e07acfa7e50fdeef1106801bad98bff07fd0b7657a07a5b3b632318b1ebd03820f7c2b6ad409e8adf6aff3bf00fc6b091efd90a9aa730e45bb6449fa943e6239db026e0e7dfcd38f2a88435245fdfdae89e79feb03194d077f5a5bef9283b22a4b4f9ede189c50d014a844080846354a02321b19df76f7daf7aefb478e57abf289dcf646558a9d605a583fd53f2041a08795278f6aece6ae88391944cfb2c2e5a0cbf365b11c6e7c4960ae883a3140039ff2c7552946745a2951acadba8815c79e0b5fa13fa353d945796647cf7aefa614675536c1c1e36043d8d12fe1e8180c1d7922255c7c29ff4bdb2ad169e3afca545d657345811f076d3bc9d3f5aa9d08b82b962195b9a282c615c92b2a2f7c4d04c2581c7c9560fb9291ece0a07e8724cac577b20855ddd4a8d149844577c5744f3c73464a1c84d7efd4b1ec662afc6f84bab07435e24301de8ba5251592d20013a70c30517037d59cd214a4f53071d764b8d8a1a018a83e5a6994e8aba0e27694c086f3fe6bd902c61126e537311dd05e7ce608808ba9c5aaec434bf0ac6d2b6d91786dfa1095c092690e54b7c081d66e81e9a7bb183b0c8e284d1997a9653fe637ba206db2c3af610cfb6919c43b9ed87d92908be8094adf2e1956f128125cc733d8fabd533df4a5a2947a532cda6d42f7c41c5a9e9302f15e228bbc819e1cb42139a1f0494f33384c29f8965bbd564b7cada5f300f7ceca32e2b42a41a7e86c78e5654acc26ed0b384c6740e99da11322ce9c1e53a68cdc8eadb9a7e9a00e922351fc5495eb6d345efd11e5aa4446d009dbcb92db2774ab2f155c90fc16a83901487540d145f1d2522dd212faac92f996ca188bf5c9848ace22ae80a23a44789e9b28464e100378f95233ad9033b68e4d1a7a506901c0c8097a047875234703c50502d49a7b3aaff3ce89c16946a3aa58e179c70a7a5998794515c9df55e366abf6776cb5235aca80447fcce18ad3ef70d8b4b5abf80b9410e21e536b5613be4feaeaf204c7fd3358fc9c00721881d174278128227ec674f37f7fe97b6d"
)

// TestDecapsulateHybridMatchesNoble is the interop test that matters: given
// noble's ciphertext and seed, Go must arrive at noble's shared secret. It
// covers the seed expansion, both KEM halves and the SHA3-256 combiner at
// once, since any one of them being wrong changes the output.
func TestDecapsulateHybridMatchesNoble(t *testing.T) {
	got, err := DecapsulateHybrid(mustHex(t, vectorCiphertext), mustHex(t, vectorSeed))
	if err != nil {
		t.Fatalf("DecapsulateHybrid: %v", err)
	}

	if want := mustHex(t, vectorSharedSecret); !bytes.Equal(got, want) {
		t.Errorf("shared secret\n got %X\nwant %X", got, want)
	}
}

// TestDecryptKeysHybridMatchesNoble exercises the whole chain the mail path
// uses: decapsulate, then AES-KW unwrap, ending at the session key.
func TestDecryptKeysHybridMatchesNoble(t *testing.T) {
	got, err := DecryptKeysHybrid(HybridEncryptedKey{
		HybridCiphertext: mustHex(t, vectorCiphertext),
		EncryptedKey:     mustHex(t, vectorWrappedKey),
	}, mustHex(t, vectorSeed))
	if err != nil {
		t.Fatalf("DecryptKeysHybrid: %v", err)
	}

	if want := mustHex(t, vectorSessionKey); !bytes.Equal(got, want) {
		t.Errorf("session key\n got %X\nwant %X", got, want)
	}
}

func TestDecapsulateHybridRejectsWrongSeed(t *testing.T) {
	wrongSeed := make([]byte, seedLen)

	got, err := DecapsulateHybrid(mustHex(t, vectorCiphertext), wrongSeed)
	if err != nil {
		// ML-KEM is designed not to fail on a wrong key, but X25519 can reject
		// a low-order point, so an error here is acceptable too.
		return
	}
	if want := mustHex(t, vectorSharedSecret); bytes.Equal(got, want) {
		t.Fatal("a different seed produced the same shared secret")
	}
}

func TestDecapsulateHybridRejectsMalformedInput(t *testing.T) {
	seed := mustHex(t, vectorSeed)
	ciphertext := mustHex(t, vectorCiphertext)

	t.Run("short seed", func(t *testing.T) {
		if _, err := DecapsulateHybrid(ciphertext, seed[:seedLen-1]); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("short ciphertext", func(t *testing.T) {
		if _, err := DecapsulateHybrid(ciphertext[:hybridCiphertextLen-1], seed); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

// nobleRecipientPublicKey is the public key noble's keygen(vectorSeed)
// produces, generated once with the real @noble/post-quantum library:
//
//	const keys = hybridCipher.keygen(seed); // seed = 0x00..0x1f
//
// vectorSeed (see above) is that same keypair's private key, already proven
// interoperable: TestDecapsulateHybridMatchesNoble decapsulates a ciphertext
// noble generated for this exact keypair.
const nobleRecipientPublicKey = "6f54098a0a0e641146614b6960ba60d8603d62f447f9ab499b47bd6906cc40b061d8634a3e88906f284958e7441ca6c725cbb97095b7671a462b6681c9e6580bbc8d60b149fa60261043afbba52f205a6028384851596adf371abea98d3347383d2bb673438f6783612bf87014f7b91a89740265345df679340473d1c4c176886e5e29b8f058bb7c735316686cff5c3beb8c261cb00970a69c1afcc54b94cb86e1ce63ba636e395ca45101e21c7bd04c313ea19af24141efd2ad44416a25ba4f65910ef7d8809c3093f04aaf00e3cd96e35c4aa3c802c18ad6f39da4b4b8d98c8bd7902d83a07ba45396674a60243cab93e80fd9b1c8777376a9cc0d6fa115e2639380b9c6be7848bd13588c64703a0535d19a0f81633a976a0a105b66ee285d0fd255e82c0331925f4383b6efc761ef6099235a0b98726358aa9d01b8b896519f921474bb7c14bb22252b5c2f10d41246c9b23e7644849367f541a15f63bc928a39bb7bc73f07b665c496bb6558c8f45489a72ec4bacd34e9c594c33871b723f03495e88b4391ab26e43043deb6117b3919e45c4c1b16ab28e47ddd723663854766192fc1806ca70abb786cbdb30932e68c8a370bcfb07983a012c3266b93efa62657f4b838374cb0bb95e0ec06541b0765d99cf153bc6b96135ca780a55b3647789e31915e46283cf9c7bb6e8453fb6682105141f1dc0d00d85eed703b6c6c961f79c845276b4248949c06782e513eb2991b95d96042e38cbeda352449b2b5084ebda5226a6206400789130a3096449848b629feea4a2c2a743c4a0ddc9cb3f3d676fc563731b26c4a1a66dc8459170056d57697f1443b81a9a34412bb7bf05f3327575a5911dd301d6053867f3c3080711f1bf11587b0bb2984276b2685e7756210e4b3f8955384231e558c6f510c91e0fc56b5d1885ff2949e95a46bc1bee1fa71f5027e10c443b0e91d0fd7440f467a27221212e88f5c6ba64296cae0d207bfc60f88c7cfb5c45aa1839d18cb37c45843e5426a4a90c802b6428f953c359c4ac0603452fac0b7361e2fd35dcc885a92145d4fca0158f1b7d70b4bcd118e4a2a4154438df310c44a9a1b99ea415907267a88b0624241579c1722f46ed61c2e3eca545c9970517175399b800db25da39593d06490d7142c00e88d2db047e9898bdb7acb7ed907f6e30416cc0de54a242c0a2126302f5d54c85bc66ac2f83c797945b5067caa42bd2e0c19ca97506e507ab0a5c9f5633708499c19f24aec513bd3903a5d73b6ec4991f7c72eb991c1c37889805cb1ea38a0cc02176b27c58d638ce5a32668457cf9b9be027ca0214057971725d54102e8996716eb2ad823453b605b855370b1b21b3932cded4160aa9973c7ebae5ac4764d94cf7cc9506f077bad73012dbb4ac8140a38746412eb33c9514596205f707635862217d9b60918c6268d9344915b847a2476c1a270f154a5c84234165acfc869398702cea9e9a07e7b0e99ea9bdcb7841fe9c0fa25c8338092561a3edddc7001f478ad65781a6024aad165d9b6979adac448a4462f564685527f762434fe9a425a84437b457392eca80c913506151e3a13239f342fca7655b6eaae845a221ceb3e67f5639c6193f6fdeef57e399b808b7f3aa2b5740aaded90163dc5d775c9faf7f1fbd075dab344e9d7d146647281fbba7b3c56cafd5833b7a930ec4206e7c3a6d7764fe81d7a"

// TestEncapsulateHybridDecapsulatesToItself is a plain round trip within Go:
// encapsulating against a public key and decapsulating the resulting
// ciphertext against the matching private key must agree on the same shared
// secret.
func TestEncapsulateHybridDecapsulatesToItself(t *testing.T) {
	publicKey := mustHex(t, nobleRecipientPublicKey)

	ciphertext, sharedSecret, err := EncapsulateHybrid(publicKey)
	if err != nil {
		t.Fatalf("EncapsulateHybrid: %v", err)
	}
	if len(ciphertext) != hybridCiphertextLen {
		t.Fatalf("ciphertext is %d bytes, want %d", len(ciphertext), hybridCiphertextLen)
	}

	got, err := DecapsulateHybrid(ciphertext, mustHex(t, vectorSeed))
	if err != nil {
		t.Fatalf("DecapsulateHybrid: %v", err)
	}
	if !bytes.Equal(got, sharedSecret) {
		t.Errorf("shared secret\n got %X\nwant %X", got, sharedSecret)
	}
}

// TestEncapsulateHybridMatchesNobleKey is the interop check: it encapsulates
// in Go against a public key noble itself generated (nobleRecipientPublicKey),
// then decapsulates with the matching private key (vectorSeed) using our
// DecapsulateHybrid, already proven to agree with noble on real ciphertexts.
//
// Encapsulate is randomised — an ephemeral X25519 key plus ML-KEM randomness —
// so there is no fixed ciphertext to pin, the way there is for decapsulate.
// This is the strongest check available instead: a keypair noble actually
// produced, opened correctly by the half of this package noble's own output
// already validates.
func TestEncapsulateHybridMatchesNobleKey(t *testing.T) {
	publicKey := mustHex(t, nobleRecipientPublicKey)
	privateKey := mustHex(t, vectorSeed)

	for i := 0; i < 5; i++ {
		ciphertext, sharedSecret, err := EncapsulateHybrid(publicKey)
		if err != nil {
			t.Fatalf("EncapsulateHybrid: %v", err)
		}

		got, err := DecapsulateHybrid(ciphertext, privateKey)
		if err != nil {
			t.Fatalf("DecapsulateHybrid: %v", err)
		}
		if !bytes.Equal(got, sharedSecret) {
			t.Fatalf("run %d: shared secret\n got %X\nwant %X", i, got, sharedSecret)
		}
	}
}

func TestEncapsulateHybridRejectsMalformedInput(t *testing.T) {
	publicKey := mustHex(t, nobleRecipientPublicKey)

	if _, _, err := EncapsulateHybrid(publicKey[:hybridPublicKeyLen-1]); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestEncryptKeysHybridRoundTripsWithDecrypt covers the whole chain in the
// encrypt direction: wrap a session key for a recipient, then recover it with
// DecryptKeysHybrid.
func TestEncryptKeysHybridRoundTripsWithDecrypt(t *testing.T) {
	publicKey := mustHex(t, nobleRecipientPublicKey)
	privateKey := mustHex(t, vectorSeed)
	sessionKey := mustHex(t, vectorSessionKey)

	encrypted, err := EncryptKeysHybrid(sessionKey, publicKey)
	if err != nil {
		t.Fatalf("EncryptKeysHybrid: %v", err)
	}

	got, err := DecryptKeysHybrid(HybridEncryptedKey{
		HybridCiphertext: encrypted.HybridCiphertext,
		EncryptedKey:     encrypted.EncryptedKey,
	}, privateKey)
	if err != nil {
		t.Fatalf("DecryptKeysHybrid: %v", err)
	}
	if !bytes.Equal(got, sessionKey) {
		t.Errorf("session key\n got %X\nwant %X", got, sessionKey)
	}
}

// TestExpandHybridSeedSplit guards the sizes the wire format depends on.
func TestExpandHybridSeedSplit(t *testing.T) {
	mlkemSeed, x25519Secret := expandHybridSeed(mustHex(t, vectorSeed))

	if len(mlkemSeed) != mlkemSeedLen {
		t.Errorf("ML-KEM seed is %d bytes, want %d", len(mlkemSeed), mlkemSeedLen)
	}
	if len(x25519Secret) != x25519KeyLen {
		t.Errorf("X25519 secret is %d bytes, want %d", len(x25519Secret), x25519KeyLen)
	}
}
