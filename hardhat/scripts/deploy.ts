import { ethers } from "hardhat";

async function main() {
  const [deployer] = await ethers.getSigners();
  
  console.log("Deploying contracts with the account:", deployer.address);

  const balance = await ethers.provider.getBalance(deployer.address);
  
  console.log("Account balance:", ethers.formatEther(balance), "ETH");

  // Deploy your contract
  const Contract = await ethers.getContractFactory("TwoPhaseCommit"); // Replace with your contract name
  const contract = await Contract.deploy(); // Deploy the contract (add constructor arguments if needed)

  await contract.waitForDeployment();

  console.log("Contract deployed to:", await contract.getAddress());
}

// Run the script
main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
