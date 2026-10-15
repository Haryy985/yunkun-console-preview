import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "云鲲自动化渗透控制台",
  shortName: "云鲲控制台",
  team: "云鲲安全",
  version: packageJson.version,
  copyright: `© ${currentYear}, 云鲲安全.`,
  meta: {
    title: "云鲲自动化渗透控制台",
    description: "云鲲安全任务与资产工作台",
  },
};
