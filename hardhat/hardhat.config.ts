import * as dotenv from "dotenv";
dotenv.config();

import { HardhatUserConfig } from "hardhat/config";
import "@nomicfoundation/hardhat-ethers";
// hardhat.config.ts
import "@nomicfoundation/hardhat-chai-matchers/withArgs";
import "@nomicfoundation/hardhat-chai-matchers";

const config: HardhatUserConfig = {
  solidity: {
    compilers: [
      {
        version: "0.8.28", // For Lock.sol
      },
      {
        version: "0.8.19", // For Master.sol
      },
    ],
  },
  networks: {
    fuji: {
      url: "https://api.avax-test.network/ext/bc/C/rpc",
      accounts: [process.env.PRIVATE_KEY as string],
      chainId: 43113,
    },
  },
};

export default config;
