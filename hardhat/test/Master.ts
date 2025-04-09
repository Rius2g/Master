import { ethers } from "hardhat";
import { expect } from "chai";
import { LamportClock } from "../typechain-types";
import { toUtf8Bytes, keccak256, hexlify } from "ethers";

describe("LamportClock", function () {
  let lamportClock: LamportClock;
  let owner: any;
  let alice: any;
  let bob: any;

  beforeEach(async function () {
    [owner, alice, bob] = await ethers.getSigners();
    const LamportClockFactory = await ethers.getContractFactory("LamportClock");
    lamportClock = (await LamportClockFactory.deploy()) as LamportClock;
    await lamportClock.waitForDeployment();
  });

  it("Should get dataId = 0 when no data is set", async function () {
    expect(await lamportClock.getCurrentDataId()).to.equal(0);
  });

  it("Should get dataId = 1 when data is set", async function () {
    const stringValue = "test message";
    const bytesValue = toUtf8Bytes(stringValue);
    const dependencies: string[] = [];

    await lamportClock.publishMessage(
      bytesValue,
      "Alice",
      "Test data",
      dependencies
    );

    expect(await lamportClock.getCurrentDataId()).to.equal(1);

    const data = await lamportClock.getMissingDataItems(0);
    expect(data.length).to.equal(1);

    const dataItem = data[0];
    expect(dataItem.dataId).to.equal(1);
    expect(dataItem.data).to.equal(hexlify(bytesValue));
    expect(dataItem.owner).to.equal("Alice");
    expect(dataItem.dataName).to.equal("Test data");
    // We don't check exact timestamp but ensure it's a recent timestamp
    expect(dataItem.messageTimestamp).to.be.above(0);
  });

  it("Should get the correct dataID and items based on dataID set", async function () {
    const stringValue = "first message";
    const bytesValue = toUtf8Bytes(stringValue);
    const dependencies: string[] = [];

    await lamportClock.publishMessage(
      bytesValue,
      "Alice",
      "Message 1",
      dependencies
    );

    const stringValue2 = "second message";
    const bytesValue2 = toUtf8Bytes(stringValue2);

    await lamportClock.publishMessage(
      bytesValue2,
      "Bob",
      "Message 2",
      dependencies
    );

    expect(await lamportClock.getCurrentDataId()).to.equal(2);

    const data = await lamportClock.getMissingDataItems(0);
    expect(data.length).to.equal(2);

    const dataItem = data[0];
    expect(dataItem.dataId).to.equal(1);
    expect(dataItem.data).to.equal(hexlify(bytesValue));
    expect(dataItem.owner).to.equal("Alice");
    expect(dataItem.dataName).to.equal("Message 1");

    const dataItem2 = data[1];
    expect(dataItem2.dataId).to.equal(2);
    expect(dataItem2.data).to.equal(hexlify(bytesValue2));
    expect(dataItem2.owner).to.equal("Bob");
    expect(dataItem2.dataName).to.equal("Message 2");

    const data2 = await lamportClock.getMissingDataItems(1);
    expect(data2.length).to.equal(1);
    expect(data2[0].dataId).to.equal(2);
    expect(data2[0].data).to.equal(hexlify(bytesValue2));
    expect(data2[0].owner).to.equal("Bob");
    expect(data2[0].dataName).to.equal("Message 2");
  });

  it("Publishing a message should emit an event", async function () {
    const stringValue = "event test message";
    const bytesValue = toUtf8Bytes(stringValue);
    const dependencies: string[] = [];

    const tx = await lamportClock.publishMessage(
      bytesValue,
      "Alice",
      "Event Test",
      dependencies
    );

    await tx.wait();

    // Verify event was emitted
    const events = await lamportClock.queryFilter(
      lamportClock.filters.BroadcastMessage()
    );
    expect(events.length).to.equal(1);
    const event = events[0];
    expect(event.args.data).to.equal(hexlify(bytesValue));
    expect(event.args.owner).to.equal("Alice");
    expect(event.args.dataName).to.equal("Event Test");
    expect(event.args.dataId).to.equal(1);
    expect(event.args.dependencies).to.deep.equal(dependencies);
  });

  it("Should revert if any of the strings are empty", async function () {
    const stringValue = "validation test";
    const bytesValue = toUtf8Bytes(stringValue);
    const dependencies: string[] = [];

    await expect(
      lamportClock.publishMessage(bytesValue, "Alice", "", dependencies)
    ).to.be.revertedWith("Data name is required");

    await expect(
      lamportClock.publishMessage(bytesValue, "", "Test data", dependencies)
    ).to.be.revertedWith("Owner is required");

    // Test empty data
    await expect(
      lamportClock.publishMessage(
        new Uint8Array(0),
        "Alice",
        "Test data",
        dependencies
      )
    ).to.be.revertedWith("Data is required");
  });

  describe("Dependency Handling", function () {
    it("Should enforce message dependencies correctly", async function () {
      // First message
      const msg1 = toUtf8Bytes("first message");

      // Store first message
      const tx1 = await lamportClock.publishMessage(
        msg1,
        "Alice",
        "Message1",
        [] // No dependencies
      );
      await tx1.wait();

      // Get the stored message to get its actual hash
      const data = await lamportClock.getMissingDataItems(0);
      const msg1StoredData = data[0];
      const msg1ActualHash = keccak256(
        ethers.solidityPacked(["bytes"], [msg1StoredData.data])
      );

      console.log("Hash Calculation:", {
        original: hexlify(msg1),
        stored: msg1StoredData.data,
        calculatedHash: msg1ActualHash,
      });

      // Debug check the dependency
      const debugTx = await lamportClock.debugCheckDependency(msg1ActualHash);
      await debugTx.wait();

      const [isValid, hashes] = await lamportClock.checkDependency(
        msg1ActualHash
      );
      console.log("Dependency Check:", {
        msg1Hash: msg1ActualHash,
        contractHashes: hashes,
        isValid: isValid,
      });
      expect(isValid).to.be.true;

      // Second message depending on first
      const msg2 = toUtf8Bytes("second message");
      await lamportClock.publishMessage(
        msg2,
        "Bob",
        "Message2",
        [msg1ActualHash] // Depend on first message
      );

      // Try with invalid dependency - should fail
      const invalidHash = keccak256(toUtf8Bytes("non-existent message"));
      await expect(
        lamportClock.publishMessage(
          toUtf8Bytes("invalid dependency message"),
          "Charlie",
          "Invalid Dependency",
          [invalidHash]
        )
      ).to.be.revertedWith("Dependency not found");

      // Create a chain of dependencies
      const msg3 = toUtf8Bytes("third message");
      await lamportClock.publishMessage(
        msg3,
        "Charlie",
        "Message3",
        [msg1ActualHash] // Also depends on first message
      );

      // Get the hash for the second message
      const updatedData = await lamportClock.getMissingDataItems(0);
      const msg2StoredData = updatedData[1]; // Second message
      const msg2ActualHash = keccak256(
        ethers.solidityPacked(["bytes"], [msg2StoredData.data])
      );

      // Create a message that depends on both previous messages
      const msg4 = toUtf8Bytes("fourth message");
      await lamportClock.publishMessage(
        msg4,
        "Dave",
        "Message4",
        [msg1ActualHash, msg2ActualHash] // Depends on first and second messages
      );

      // Verify the dependencies were stored correctly
      const finalData = await lamportClock.getMissingDataItems(0);
      const msg4Data = finalData[3]; // Fourth message
      expect(msg4Data.dependencies.length).to.equal(2);
      expect(msg4Data.dependencies).to.include(msg1ActualHash);
      expect(msg4Data.dependencies).to.include(msg2ActualHash);
    });

    it("Should correctly track vector clocks", async function () {
      // First message from Alice
      const msg1 = toUtf8Bytes("alice message");
      await lamportClock
        .connect(alice)
        .publishMessage(msg1, "Alice", "Alice Message", []);

      // Second message from Bob
      const msg2 = toUtf8Bytes("bob message");
      await lamportClock
        .connect(bob)
        .publishMessage(msg2, "Bob", "Bob Message", []);

      // Get the messages
      const data = await lamportClock.getMissingDataItems(0);

      // Check Alice's vector clock
      const aliceMsg = data[0];
      expect(aliceMsg.vectorClocks.length).to.equal(1);
      expect(aliceMsg.vectorClocks[0].process).to.equal(alice.address);
      expect(aliceMsg.vectorClocks[0].timeStamp).to.be.above(0);

      // Check Bob's vector clock
      const bobMsg = data[1];
      expect(bobMsg.vectorClocks.length).to.equal(1);
      expect(bobMsg.vectorClocks[0].process).to.equal(bob.address);
      expect(bobMsg.vectorClocks[0].timeStamp).to.be.above(0);

      // Get hashes for dependencies
      const aliceMsgHash = keccak256(
        ethers.solidityPacked(["bytes"], [aliceMsg.data])
      );

      // Third message from Alice that depends on Bob's message
      const msg3 = toUtf8Bytes("alice reply to bob");
      const bobMsgHash = keccak256(
        ethers.solidityPacked(["bytes"], [bobMsg.data])
      );

      await lamportClock
        .connect(alice)
        .publishMessage(msg3, "Alice", "Alice Reply", [bobMsgHash]);

      // Fourth message from Bob that depends on Alice's first message
      const msg4 = toUtf8Bytes("bob reply to alice");
      await lamportClock
        .connect(bob)
        .publishMessage(msg4, "Bob", "Bob Reply", [aliceMsgHash]);

      // Verify events contain vector clocks
      const events = await lamportClock.queryFilter(
        lamportClock.filters.BroadcastMessage()
      );
      expect(events.length).to.equal(4);

      // Check the latest messages
      const aliceReply = events[2];
      const bobReply = events[3];

      expect(aliceReply.args.vectorClocks.length).to.equal(1);
      expect(aliceReply.args.vectorClocks[0].process).to.equal(alice.address);

      expect(bobReply.args.vectorClocks.length).to.equal(1);
      expect(bobReply.args.vectorClocks[0].process).to.equal(bob.address);

      // Verify dependency tracking in events
      expect(aliceReply.args.dependencies.length).to.equal(1);
      expect(aliceReply.args.dependencies[0]).to.equal(bobMsgHash);

      expect(bobReply.args.dependencies.length).to.equal(1);
      expect(bobReply.args.dependencies[0]).to.equal(aliceMsgHash);
    });
  });

  it("Should handle long dependency chains correctly", async function () {
    // Create a chain of 5 messages where each depends on the previous one
    const messages = [
      "first in chain",
      "second in chain",
      "third in chain",
      "fourth in chain",
      "fifth in chain",
    ];

    let previousHash: string | null = null;

    // Create the chain of messages
    for (let i = 0; i < messages.length; i++) {
      const msgBytes = toUtf8Bytes(messages[i]);
      const dependencies = previousHash ? [previousHash] : [];

      await lamportClock.publishMessage(
        msgBytes,
        `Sender${i}`,
        `Message${i + 1}`,
        dependencies
      );

      // Get the hash of this message for the next one to depend on
      const data = await lamportClock.getMissingDataItems(i);
      const latestMsg = data[data.length - 1];
      previousHash = keccak256(
        ethers.solidityPacked(["bytes"], [latestMsg.data])
      );
    }

    // Verify the full chain was created
    const allData = await lamportClock.getMissingDataItems(0);
    expect(allData.length).to.equal(5);

    // Try to create a message that depends on all messages in the chain
    // This tests handling multiple dependencies
    const finalMessage = toUtf8Bytes("depends on all");

    // Get all the hashes
    const dependencies = allData.map((item) =>
      keccak256(ethers.solidityPacked(["bytes"], [item.data]))
    );

    // This should succeed because all dependencies exist
    await lamportClock.publishMessage(
      finalMessage,
      "FinalSender",
      "FinalMessage",
      dependencies
    );

    // Verify final message was stored with all dependencies
    const finalData = await lamportClock.getMissingDataItems(0);
    const finalItem = finalData[finalData.length - 1];
    expect(finalItem.dependencies.length).to.equal(5);

    // Each dependency should match our collected hashes
    for (const hash of dependencies) {
      expect(finalItem.dependencies).to.include(hash);
    }
  });

  it("Should detect concurrent messages correctly", async function () {
    // Create two independent messages from different processes
    const msgA = toUtf8Bytes("concurrent message A");
    const msgB = toUtf8Bytes("concurrent message B");

    // Send messages concurrently
    await Promise.all([
      lamportClock
        .connect(alice)
        .publishMessage(msgA, "Alice", "ConcurrentA", []),
      lamportClock.connect(bob).publishMessage(msgB, "Bob", "ConcurrentB", []),
    ]);

    // Get the messages
    const data = await lamportClock.getMissingDataItems(0);

    // Since these are concurrent, neither should depend on the other
    expect(data[0].dependencies.length).to.equal(0);
    expect(data[1].dependencies.length).to.equal(0);

    // But they should have different vector clocks
    expect(data[0].vectorClocks[0].process).to.not.equal(
      data[1].vectorClocks[0].process
    );
  });
});
