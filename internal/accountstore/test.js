const crypto = require('crypto');

const hashWithSalt = (password, salt) => {
  return new Promise((resolve, reject) => {
    crypto.scrypt(password, salt, 64, (err, hash) => {
      if (err) return reject(err)
      resolve(salt + ':' + hash.toString('hex'))
    })
  })
}

const verify = async (password, storedHash) => {
  const [salt, hash] = storedHash.split(':')
  const derivedHash = await getDerivedHash(password, salt)
  return hash === derivedHash
}

const getDerivedHash = (password, salt) => {
  return new Promise((resolve, reject) => {
    crypto.scrypt(password, salt, 64, (err, derivedHash) => {
      if (err) return reject(err)
      resolve(derivedHash.toString('hex'))
    })
  })
}

let salt = "ba59d820fc6b2203fc5d212852fe9809";
// const salt = "abc123";
// const hash = 'de019851402f8b727305780ea4453c63ae8066fff6987626d8fc4f80667a7bc270b766ab445e0b447e7ae9fdf2018c1d19b1e3cf2b4770d00a47c6e9d2a3a0de';
const hash = "de019851402f8b727305780ea4453c63ae8066fff6987626d8fc4f80667a7bc270b766ab445e0b447e7ae9fdf2018c1d19b1e3cf2b4770d00a47c6e9d2a3a0de";

const password = process.env["BSKY_TEST_PASSWORD"];
if (!password) {
  throw Error("no test password");
}

console.log(Buffer.from(password));

crypto.scrypt(
  password,
	// Buffer.from(salt, "hex"),
	salt,
  64,
  // {
  //   N: 1 << 14,
  //   r: 8,
  //   p: 1,
  //   // maxmem: 32 << 20,
  //   maxmem: 32 << 20,
  // },
  (err, result) => {
    if (err) throw err;
    // console.log('hash hex:', result.toString('hex'));
    // console.log(result.toString('hex') === hash);
		console.log("expected:", hash);
		console.log("got:     ", result.toString("hex"));
  },
);

getDerivedHash(password, Buffer.from(salt).toString()).then((hash) => {
	console.log("hash:", hash);
});

verify(password, "ba59d820fc6b2203fc5d212852fe9809:de019851402f8b727305780ea4453c63ae8066fff6987626d8fc4f80667a7bc270b766ab445e0b447e7ae9fdf2018c1d19b1e3cf2b4770d00a47c6e9d2a3a0de").then((ok) => console.log("password good:", ok));

// let pass = "testpass01";
// salt = "abc12";
// crypto.scrypt(
// 	pass,
// 	salt,
// 	64,
// 	{
// 		N: 1 << 14,
// 		r: 8,
// 		p: 1,
// 		maxmem: 32 << 20,
// 	},
// 	(err, result) => {
//     if (err) throw err;
// 		console.log("test pass:", pass);
// 		console.log("test salt:", salt);
// 		console.log("test hash:", result.toString("hex"));
// 	}
// );
