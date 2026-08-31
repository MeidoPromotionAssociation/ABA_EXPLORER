/** toBigInt 把后端给的十进制字符串转成 bigint，非法值回落到 0 以免整表排序崩掉 */
export function toBigInt(value: string): bigint {
    try {
        return BigInt(value);
    } catch {
        return 0n;
    }
}
