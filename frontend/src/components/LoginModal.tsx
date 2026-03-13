import React, { useState, useEffect } from 'react';
import './LoginModal.css';

interface Props {
  onSuccess: (sessionToken: string) => void;
}

// Pure-JS SHA-256 for environments without crypto.subtle (plain HTTP on LAN)
function sha256Bytes(data: Uint8Array): Uint8Array {
  const H = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19];
  const K = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
  ];
  const rotr = (n: number, b: number) => ((n >>> b) | (n << (32 - b))) >>> 0;
  const msgLen = data.length;
  const padded = new Uint8Array(Math.ceil((msgLen + 9) / 64) * 64);
  padded.set(data);
  padded[msgLen] = 0x80;
  const dv = new DataView(padded.buffer as ArrayBuffer);
  const bitLen = msgLen * 8;
  dv.setUint32(padded.length - 8, Math.floor(bitLen / 0x100000000));
  dv.setUint32(padded.length - 4, bitLen >>> 0);
  for (let offset = 0; offset < padded.length; offset += 64) {
    const W = new Uint32Array(64);
    for (let i = 0; i < 16; i++) W[i] = dv.getUint32(offset + i * 4);
    for (let i = 16; i < 64; i++) {
      const s0 = rotr(W[i - 15], 7) ^ rotr(W[i - 15], 18) ^ (W[i - 15] >>> 3);
      const s1 = rotr(W[i - 2], 17) ^ rotr(W[i - 2], 19) ^ (W[i - 2] >>> 10);
      W[i] = (W[i - 16] + s0 + W[i - 7] + s1) >>> 0;
    }
    let [a, b, c, d, e, f, g, h] = H;
    for (let i = 0; i < 64; i++) {
      const S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
      const ch = (e & f) ^ (~e & g);
      const temp1 = (h + S1 + ch + K[i] + W[i]) >>> 0;
      const S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const temp2 = (S0 + maj) >>> 0;
      [h, g, f, e, d, c, b, a] = [g, f, e, (d + temp1) >>> 0, c, b, a, (temp1 + temp2) >>> 0];
    }
    H[0] = (H[0] + a) >>> 0; H[1] = (H[1] + b) >>> 0;
    H[2] = (H[2] + c) >>> 0; H[3] = (H[3] + d) >>> 0;
    H[4] = (H[4] + e) >>> 0; H[5] = (H[5] + f) >>> 0;
    H[6] = (H[6] + g) >>> 0; H[7] = (H[7] + h) >>> 0;
  }
  const result = new Uint8Array(32);
  const rv = new DataView(result.buffer as ArrayBuffer);
  H.forEach((val, i) => rv.setUint32(i * 4, val));
  return result;
}

function hmacSha256Fallback(keyStr: string, msgStr: string): string {
  const enc = new TextEncoder();
  let key = enc.encode(keyStr);
  const msg = enc.encode(msgStr);
  const BLOCK = 64;
  if (key.length > BLOCK) key = sha256Bytes(key) as Uint8Array<ArrayBuffer>;
  const ipad = new Uint8Array(BLOCK);
  const opad = new Uint8Array(BLOCK);
  for (let i = 0; i < BLOCK; i++) {
    ipad[i] = (key[i] || 0) ^ 0x36;
    opad[i] = (key[i] || 0) ^ 0x5c;
  }
  const inner = new Uint8Array(BLOCK + msg.length);
  inner.set(ipad); inner.set(msg, BLOCK);
  const innerHash = sha256Bytes(inner);
  const outer = new Uint8Array(BLOCK + 32);
  outer.set(opad); outer.set(innerHash, BLOCK);
  return Array.from(sha256Bytes(outer)).map(b => b.toString(16).padStart(2, '0')).join('');
}

async function computeHmac(key: string, message: string): Promise<string> {
  if (window.isSecureContext && typeof crypto !== 'undefined' && crypto.subtle) {
    const enc = new TextEncoder();
    const k = await crypto.subtle.importKey('raw', enc.encode(key), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
    const sig = await crypto.subtle.sign('HMAC', k, enc.encode(message));
    return Array.from(new Uint8Array(sig)).map(b => b.toString(16).padStart(2, '0')).join('');
  }
  return hmacSha256Fallback(key, message);
}

const LoginModal: React.FC<Props> = ({ onSuccess }) => {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const input = document.getElementById('playerr-password-input');
    if (input) input.focus();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      // Step 1: fetch challenge nonce
      const challengeRes = await fetch('/api/v3/auth/challenge');
      if (!challengeRes.ok) throw new Error('Could not fetch challenge');
      const { nonce } = await challengeRes.json();

      // Step 2: HMAC-SHA256(password, nonce)
      const response = await computeHmac(password, nonce);

      // Step 3: verify CHAP response → receive session token
      const verifyRes = await fetch('/api/v3/auth/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ nonce, response }),
      });
      const data = await verifyRes.json();
      if (verifyRes.ok && data.token) {
        sessionStorage.setItem('playerrAuth', data.token);
        onSuccess(data.token);
      } else {
        setError('Incorrect password.');
      }
    } catch {
      setError('Could not reach server.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-mask">
      <form className="login-modal" onSubmit={handleSubmit}>
        <h2>Playerr</h2>
        <p>Enter your password to continue.</p>
        <input
          id="playerr-password-input"
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="Password"
          autoComplete="current-password"
        />
        {error && <div className="login-error">{error}</div>}
        <button className="login-btn" type="submit" disabled={loading || !password}>
          {loading ? 'Verifying…' : 'Sign In'}
        </button>
      </form>
    </div>
  );
};

export default LoginModal;
