import { ethers } from "hardhat";
import { expect } from "chai";
import { TwoPhaseDissemination } from "../typechain-types";
import { time } from "@nomicfoundation/hardhat-network-helpers";
import { toUtf8Bytes, keccak256, hexlify } from "ethers";

describe("TwoPhaseDissemination", function () {
  let twoPhaseDiss: TwoPhaseDissemination;
  let owner: any;
  let alice: any;
  let bob: any;

  beforeEach(async function () {
    [owner, alice, bob] = await ethers.getSigners();
    const TwoPhaseDissFactory = await ethers.getContractFactory(
      "TwoPhaseDissemination"
    );
    twoPhaseDiss =
      (await TwoPhaseDissFactory.deploy()) as TwoPhaseDissemination;
    await twoPhaseDiss.waitForDeployment();

    // Set up default security levels for basic tests
    await twoPhaseDiss.setProcessSecurityLevel(owner.address, 2);
    await twoPhaseDiss.setProcessSecurityLevel(alice.address, 1);
    await twoPhaseDiss.setProcessSecurityLevel(bob.address, 2);
    await twoPhaseDiss.setDataSecurityLevel("Test data", 1);
    await twoPhaseDiss.setDataSecurityLevel("Test data2", 1);
  });

  it("Should get dataId = 0 when no data is set", async function () {
    expect(await twoPhaseDiss.getCurrentDataId()).to.equal(0);
  });

  it("Should get dataId = 1 when data is set", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) + 60;
    const dependencies: string[] = [];

    await twoPhaseDiss.addStoredData(
      bytesValue,
      "Alice",
      "Test data",
      releaseTime,
      dependencies,
      1
    );

    expect(await twoPhaseDiss.getCurrentDataId()).to.equal(1);

    const data = await twoPhaseDiss.getMissingDataItems(0);
    expect(data.length).to.equal(1);

    const dataItem = data[0];
    expect(dataItem.dataId).to.equal(1);
    expect(dataItem.encryptedData).to.equal(hexlify(bytesValue));
    expect(dataItem.owner).to.equal("Alice");
    expect(dataItem.dataName).to.equal("Test data");
    expect(dataItem.releaseTime).to.equal(releaseTime);
  });

  it("Should get the correct dataID and items based on dataID set", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) + 60;
    const dependencies: string[] = [];

    await twoPhaseDiss.addStoredData(
      bytesValue,
      "Alice",
      "Test data",
      releaseTime,
      dependencies,
      1
    );

    const stringValue2 = "heia lyn2";
    const bytesValue2 = toUtf8Bytes(stringValue2);
    const releaseTime2 = (await time.latest()) + 60;

    await twoPhaseDiss.addStoredData(
      bytesValue2,
      "Alice2",
      "Test data2",
      releaseTime2,
      dependencies,
      1
    );

    expect(await twoPhaseDiss.getCurrentDataId()).to.equal(2);

    const data = await twoPhaseDiss.getMissingDataItems(0);
    expect(data.length).to.equal(2);

    const dataItem = data[0];
    expect(dataItem.dataId).to.equal(1);
    expect(dataItem.encryptedData).to.equal(hexlify(bytesValue));
    expect(dataItem.owner).to.equal("Alice");
    expect(dataItem.dataName).to.equal("Test data");
    expect(dataItem.releaseTime).to.equal(releaseTime);

    const dataItem2 = data[1];
    expect(dataItem2.dataId).to.equal(2);
    expect(dataItem2.encryptedData).to.equal(hexlify(bytesValue2));
    expect(dataItem2.owner).to.equal("Alice2");
    expect(dataItem2.dataName).to.equal("Test data2");
    expect(dataItem2.releaseTime).to.equal(releaseTime2);

    const data2 = await twoPhaseDiss.getMissingDataItems(1);
    expect(data2.length).to.equal(1);
    expect(data2[0].dataId).to.equal(2);
    expect(data2[0].encryptedData).to.equal(hexlify(bytesValue2));
    expect(data2[0].owner).to.equal("Alice2");
    expect(data2[0].dataName).to.equal("Test data2");
    expect(data2[0].releaseTime).to.equal(releaseTime2);
  });

  it("Submitting a data entry should emit an event", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) + 60;
    const dependencies: string[] = [];

    const tx = await twoPhaseDiss.addStoredData(
      bytesValue,
      "Alice",
      "Test data",
      releaseTime,
      dependencies,
      1
    );

    await tx.wait();

    // Verify event was emitted
    const events = await twoPhaseDiss.queryFilter(
      twoPhaseDiss.filters.ReleaseEncryptedData()
    );
    expect(events.length).to.equal(1);
    const event = events[0];
    expect(event.args.encryptedData).to.equal(hexlify(bytesValue));
    expect(event.args.owner).to.equal("Alice");
    expect(event.args.dataName).to.equal("Test data");
    expect(event.args.releaseTime).to.equal(releaseTime);
    expect(event.args.dataId).to.equal(1);
    expect(event.args.dependencies).to.deep.equal(dependencies);
  });

  it("should emit KeyReleaseRequested event when time is ripe", async function () {
    const currentTime = await time.latest();
    const releaseTime = currentTime + 43200;
    const dependencies: string[] = [];

    const testData = {
      encryptedData: toUtf8Bytes("heia lyn"),
      owner: "Alice",
      dataName: "Test data",
      releaseTime: releaseTime,
    };

    await twoPhaseDiss.addStoredData(
      testData.encryptedData,
      testData.owner,
      testData.dataName,
      testData.releaseTime,
      dependencies,
      1
    );

    await time.increase(43200);

    const checkData = "0x";
    const { upkeepNeeded } = await twoPhaseDiss.checkUpkeep.staticCall(
      checkData
    );
    expect(upkeepNeeded).to.equal(true);

    const tx = await twoPhaseDiss.performUpkeep(checkData);
    await expect(tx)
      .to.emit(twoPhaseDiss, "KeyReleaseRequested")
      .withArgs(0, testData.owner, testData.dataName, 1);
  });

  it("Should revert if any of the strings are empty", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) + 60;
    const dependencies: string[] = [];

    await expect(
      twoPhaseDiss.addStoredData(
        bytesValue,
        "Alice",
        "",
        releaseTime,
        dependencies,
        1
      )
    ).to.be.revertedWith("Data name is required");

    await expect(
      twoPhaseDiss.addStoredData(
        bytesValue,
        "",
        "Test data",
        releaseTime,
        dependencies,
        1
      )
    ).to.be.revertedWith("Owner is required");
  });

  it("Should revert if releaseTime is in the past", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) - 60;
    const dependencies: string[] = [];

    await expect(
      twoPhaseDiss.addStoredData(
        bytesValue,
        "Alice",
        "Test data",
        releaseTime,
        dependencies,
        1
      )
    ).to.be.revertedWith("Release time must be in the future");
  });

  it("Should revert if security level is 0", async function () {
    const stringValue = "heia lyn";
    const bytesValue = toUtf8Bytes(stringValue);
    const releaseTime = (await time.latest()) + 60;
    const dependencies: string[] = [];

    await expect(
      twoPhaseDiss.addStoredData(
        bytesValue,
        "Alice",
        "Test data",
        releaseTime,
        dependencies,
        0
      )
    ).to.be.revertedWith("Security level must be greater than 0");
  });

  describe("Security and Ordering", function () {
    it("Should allow high security process to access low security data", async function () {
      const releaseTime = (await time.latest()) + 60;
      const data = toUtf8Bytes("low security data");
      const dependencies: string[] = [];

      await expect(
        twoPhaseDiss
          .connect(bob)
          .addStoredData(
            data,
            "Bob",
            "LowSecretData",
            releaseTime,
            dependencies,
            1
          )
      ).to.not.be.reverted;
    });

    it("Should prevent low security process from accessing high security data", async function () {
      const releaseTime = (await time.latest()) + 60;
      const data = toUtf8Bytes("high security data");
      const dependencies: string[] = [];

      await expect(
        twoPhaseDiss
          .connect(alice)
          .addStoredData(
            data,
            "Alice",
            "HighSecretData",
            releaseTime,
            dependencies,
            2
          )
      ).to.be.revertedWith("Process security level is too low");
    });

    it("Should enforce message dependencies", async function () {
      const releaseTime = (await time.latest()) + 60;

      // First message
      const msg1 = toUtf8Bytes("first message");

      // Store first message
      const tx1 = await twoPhaseDiss.addStoredData(
        msg1,
        "Alice",
        "Message1",
        releaseTime,
        [], // No dependencies
        1
      );
      await tx1.wait();

      // Get the stored message to get its actual hash
      const data = await twoPhaseDiss.getMissingDataItems(0);
      const msg1StoredData = data[0];
      const msg1ActualHash = keccak256(
        ethers.solidityPacked(["bytes"], [msg1StoredData.encryptedData])
      );

      console.log("Hash Calculation:", {
        original: hexlify(msg1),
        stored: msg1StoredData.encryptedData,
        calculatedHash: msg1ActualHash,
      });

      // Debug check the dependency
      const debugTx = await twoPhaseDiss.debugCheckDependency(msg1ActualHash);
      await debugTx.wait();

      const [isValid, hashes] = await twoPhaseDiss.checkDependency(
        msg1ActualHash
      );
      console.log("Dependency Check:", {
        msg1Hash: msg1ActualHash,
        contractHashes: hashes,
        isValid: isValid,
      });

      // Second message depending on first
      const msg2 = toUtf8Bytes("second message");
      const tx2 = await twoPhaseDiss.addStoredData(
        msg2,
        "Alice",
        "Message2",
        releaseTime,
        [msg1ActualHash], // Use the actual hash from stored data
        1
      );
      await tx2.wait();

      // Verify the second message was stored correctly with dependency
      const msg2Data = await twoPhaseDiss.getMissingDataItems(1);
      console.log("Message 2 Data:", {
        dependencies: msg2Data[0].dependencies,
        matches: msg2Data[0].dependencies[0] === msg1ActualHash,
      });

      // Try to release key for Message2 without releasing Message1's key
      const key2 = toUtf8Bytes("key2");
      await expect(
        twoPhaseDiss.releaseKey("Message2", "Alice", key2)
      ).to.be.revertedWith("Not all dependenices met");

      // Now release key for Message1
      const key1 = toUtf8Bytes("key1");
      await twoPhaseDiss.releaseKey("Message1", "Alice", key1);

      // Now Message2's key should be releasable
      await expect(twoPhaseDiss.releaseKey("Message2", "Alice", key2)).to.not.be
        .reverted;
    });
  });
});
