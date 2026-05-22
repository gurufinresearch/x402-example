/**
 * Browser wauth signing — matches chain VerifyAuthorizationSignature (eth_secp256k1):
 *   digest = SHA256(proto)  (AuthorizationSignBytes, 32 bytes)
 *   MetaMask personal_sign: EIP-191 over the raw 32-byte digest (go-ethereum accounts.TextHash),
 *   then ECDSA on that 32-byte hash (no extra Keccak). Params must be 0x-hex bytes (ethers.hexlify(digest))
 *   so the length prefix is 32, not 64 (do not pass a UTF-8 string of 64 hex characters).
 *
 * Optional window.__WAUTH_PRIVATE_KEY__: same EIP-191 via ethers.Wallet.signMessage(digest).
 *
 * Protobuf wire matches facilitator/internal/cevm/proto.go EncodeAuthorization.
 */
(function (global) {
  'use strict';

  var SECP256K1_N = BigInt('0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141');

  function appendVarint(buf, v) {
    v = BigInt.asUintN(64, BigInt(v));
    while (v >= 0x80n) {
      buf.push(Number((v & 0x7fn) | 0x80n));
      v >>= 7n;
    }
    buf.push(Number(v));
  }

  function appendTag(buf, fieldNum, wireType) {
    appendVarint(buf, BigInt((fieldNum << 3) | wireType));
  }

  function appendLD(buf, fieldNum, data) {
    var d = data instanceof Uint8Array ? data : new Uint8Array(data);
    appendTag(buf, fieldNum, 2);
    appendVarint(buf, BigInt(d.length));
    for (var i = 0; i < d.length; i++) buf.push(d[i]);
  }

  function utf8Bytes(s) {
    return new TextEncoder().encode(s);
  }

  /** Proto3 string: omit if empty. */
  function appendStr(buf, fieldNum, s) {
    if (!s) return;
    var data = utf8Bytes(s);
    appendTag(buf, fieldNum, 2);
    appendVarint(buf, BigInt(data.length));
    for (var j = 0; j < data.length; j++) buf.push(data[j]);
  }

  function appendBytes(buf, fieldNum, b) {
    if (!b || b.length === 0) return;
    appendLD(buf, fieldNum, b);
  }

  /**
   * google.protobuf.Timestamp: seconds (int64), nanos (int32).
   * Matches proto.go encodeTimestamp (omit zero components).
   */
  function encodeTimestamp(date) {
    if (!(date instanceof Date) || isNaN(date.getTime())) return null;
    var sec = BigInt(Math.floor(date.getTime() / 1000));
    var ms = date.getTime() % 1000;
    if (ms < 0) ms += 1000;
    var ns = Math.floor(ms * 1e6);
    var b = [];
    if (sec !== 0n) {
      appendTag(b, 1, 0);
      appendVarint(b, sec);
    }
    if (ns !== 0) {
      appendTag(b, 2, 0);
      appendVarint(b, BigInt(ns));
    }
    return new Uint8Array(b);
  }

  function encodeCoin(c) {
    var b = [];
    appendStr(b, 1, c.denom);
    appendStr(b, 2, c.amount);
    return new Uint8Array(b);
  }

  /**
   * Parse SDK-style coin string, e.g. "100atest" or "100atest,200bdemo".
   */
  function parseCoinsNormalized(s) {
    var parts = String(s)
      .split(',')
      .map(function (x) {
        return x.trim();
      })
      .filter(Boolean);
    return parts.map(function (p) {
      var m = /^(\d+)(.+)$/.exec(p);
      if (!m) throw new Error('invalid coin: ' + p);
      return { amount: m[1], denom: m[2] };
    });
  }

  function hexToBytes(hex) {
    var h = String(hex).replace(/^0x/i, '');
    if (h.length % 2) throw new Error('invalid nonce hex length');
    var out = new Uint8Array(h.length / 2);
    for (var i = 0; i < out.length; i++) {
      out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
    }
    return out;
  }

  /**
   * Authorization protobuf bytes (memo must be "" for signing digest).
   * Nullable=false timestamps: fields 5 and 6 always present (empty inner msg if zero time).
   */
  function encodeAuthorizationBytes(fields) {
    var from = fields.from || '';
    var to = fields.to || '';
    var coins = fields.coins;
    var nonce = fields.nonce instanceof Uint8Array ? fields.nonce : hexToBytes(fields.nonce);
    var validAfter = fields.validAfter;
    var validBefore = fields.validBefore;
    var chainId = fields.chainId || '';
    var memo = fields.memo != null ? String(fields.memo) : '';

    var buf = [];
    appendStr(buf, 1, from);
    appendStr(buf, 2, to);
    for (var ci = 0; ci < coins.length; ci++) {
      appendLD(buf, 3, encodeCoin(coins[ci]));
    }
    appendBytes(buf, 4, nonce);

    var ts5 = encodeTimestamp(validAfter);
    if (ts5 === null || ts5.length === 0) ts5 = new Uint8Array(0);
    appendLD(buf, 5, ts5);

    var ts6 = encodeTimestamp(validBefore);
    if (ts6 === null || ts6.length === 0) ts6 = new Uint8Array(0);
    appendLD(buf, 6, ts6);

    appendStr(buf, 7, chainId);
    appendStr(buf, 8, memo);

    return new Uint8Array(buf);
  }

  async function sha256(bytes) {
    var h = await crypto.subtle.digest('SHA-256', bytes);
    return new Uint8Array(h);
  }

  /**
   * Same 32-byte digest as wauth types.AuthorizationSignBytes (memo cleared before marshal).
   */
  async function authorizationSignBytes(auth) {
    var va =
      auth.validAfter instanceof Date
        ? auth.validAfter
        : auth.valid_after
          ? new Date(auth.valid_after)
          : new Date(0);
    var vb =
      auth.validBefore instanceof Date
        ? auth.validBefore
        : auth.valid_before
          ? new Date(auth.valid_before)
          : new Date(0);

    var coinsStr = auth.coins;
    if (!coinsStr && auth.coinsStr) coinsStr = auth.coinsStr;

    var proto = encodeAuthorizationBytes({
      from: auth.from,
      to: auth.to,
      coins: parseCoinsNormalized(coinsStr || ''),
      nonce: auth.nonce,
      validAfter: va,
      validBefore: vb,
      chainId: auth.chain_id || auth.chainId || '',
      memo: '',
    });
    return sha256(proto);
  }

  /**
   * Enforce low-S (EIP-2), same as ethsecp256k1.NormalizeCompactSignature.
   */
  function normalizeCompactRSHex(sigHex) {
    var h = sigHex.startsWith('0x') ? sigHex.slice(2) : sigHex;
    if (h.length !== 130) throw new Error('expected 65-byte eth signature (130 hex chars)');
    var r = h.slice(0, 64);
    var s = h.slice(64, 128);
    var sb = BigInt('0x' + s);
    if (sb > SECP256K1_N / 2n) {
      sb = SECP256K1_N - sb;
    }
    var sOut = sb.toString(16);
    if (sOut.length > 64) throw new Error('normalized S too long');
    sOut = sOut.padStart(64, '0');
    return (r + sOut).toLowerCase();
  }

  /**
   * Same as MetaMask personal_sign: EIP-191 on raw digest bytes, then compact signature (low-S normalized).
   */
  async function signPersonalSignWithLocalWallet(privateKeyHex, expectedAddress, digest) {
    var w = new ethers.Wallet(privateKeyHex);
    if (w.address.toLowerCase() !== String(expectedAddress).toLowerCase()) {
      throw new Error(
        '__WAUTH_PRIVATE_KEY__ must match the connected account (' + expectedAddress + ')'
      );
    }
    var sigHex = await w.signMessage(digest);
    return normalizeCompactRSHex(sigHex);
  }

  /**
   * @param {object} auth - payload.authorization-shaped object (memo ignored for digest)
   * @returns {Promise<string>} 128-char hex (64-byte R||S), no 0x
   */
  async function signAuthorization(ethereum, address, auth) {
    if (typeof ethers === 'undefined') throw new Error('ethers global required');
    var digest = await authorizationSignBytes(auth);
    var digestHex = ethers.hexlify(digest);

    var pk =
      (typeof globalThis !== 'undefined' && globalThis.__WAUTH_PRIVATE_KEY__) ||
      (typeof window !== 'undefined' && window.__WAUTH_PRIVATE_KEY__);
    if (pk) {
      return await signPersonalSignWithLocalWallet(pk, address, digest);
    }

    if (!ethereum || typeof ethereum.request !== 'function') {
      throw new Error(
        'Connect MetaMask (personal_sign) or set window.__WAUTH_PRIVATE_KEY__ for local EIP-191 signing'
      );
    }

    try {
      var sigHex = await ethereum.request({
        method: 'personal_sign',
        params: [digestHex, address],
      });
      return normalizeCompactRSHex(sigHex);
    } catch (e) {
      var hint = e && e.message ? String(e.message) : String(e);
      throw new Error(
        'personal_sign failed (EIP-191 over raw 32-byte digest; use 0x-hex bytes from ethers.hexlify). ' +
          hint
      );
    }
  }

  global.WauthSign = {
    parseCoinsNormalized: parseCoinsNormalized,
    encodeAuthorizationBytes: encodeAuthorizationBytes,
    authorizationSignBytes: authorizationSignBytes,
    signAuthorization: signAuthorization,
    signPersonalSignWithLocalWallet: signPersonalSignWithLocalWallet,
    normalizeCompactRSHex: normalizeCompactRSHex,
  };
})(typeof window !== 'undefined' ? window : globalThis);
