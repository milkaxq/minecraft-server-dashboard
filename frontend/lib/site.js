export function isMarketingHost(host = "") {
  const name = host.toLowerCase().split(":")[0];
  return name === "playblockyard.com" || name === "www.playblockyard.com";
}
