/**
 * Type declaration for @noble/secp256k1 (signAsync with prehash: false, format: 'recovered').
 * Install with: pnpm add @noble/secp256k1
 */
declare module "@noble/secp256k1" {
  export function signAsync(
    message: Uint8Array,
    privateKey: Uint8Array,
    opts?: { prehash?: boolean; format?: "compact" | "recovered" },
  ): Promise<Uint8Array>;
}
