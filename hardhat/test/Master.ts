import {ethers} from "hardhat";
import {expect} from "chai";
import {TwoPhaseDissemination} from "../typechain-types";
import {time} from "@nomicfoundation/hardhat-network-helpers";
import { toUtf8Bytes, keccak256, hexlify } from "ethers";


describe("TwoPhaseDissemination", function () {
    let twoPhaseDiss: TwoPhaseDissemination;

    beforeEach(async function () {
        const TwoPhaseDissFactory = await ethers.getContractFactory("TwoPhaseDissemination");
        twoPhaseDiss = await TwoPhaseDissFactory.deploy() as TwoPhaseDissemination;
        await twoPhaseDiss.waitForDeployment();
    });
   
    it("Should get dataId = 0 when no data is set", async function () {
        expect(await twoPhaseDiss.getCurrentDataId()).to.equal(0);
    });

    it("Should get dataId = 1 when data is set", async function () {
        const stringValue = "heia lyn";
        const bytesValue = toUtf8Bytes(stringValue);
        const releaseTime = (await time.latest()) + 60;

        await twoPhaseDiss.addStoredData(
            bytesValue,
            "Alice",
            "Test data",
            releaseTime
        );

        
        expect(await twoPhaseDiss.getCurrentDataId()).to.equal(1);

        const data = await twoPhaseDiss.getMissingDataItems(0);
        expect(data.length).to.equal(1); //gets 1 data item since 1 is set

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
    
          await twoPhaseDiss.addStoredData(
                bytesValue,
                "Alice",
                "Test data",
                releaseTime
          );
    
          const stringValue2 = "heia lyn2";
          const bytesValue2 = toUtf8Bytes(stringValue2);
          const releaseTime2 = (await time.latest()) + 60;
    
          await twoPhaseDiss.addStoredData(
                bytesValue2,
                "Alice2",
                "Test data2",
                releaseTime2
          );
    
          expect(await twoPhaseDiss.getCurrentDataId()).to.equal(2);
    
          const data = await twoPhaseDiss.getMissingDataItems(0);
          expect(data.length).to.equal(2); //gets 2 data items since 2 is set
    
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

          expect(data2.length).to.equal(1); //gets 1 data item since 1 is set
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

        await expect(twoPhaseDiss.addStoredData(
            bytesValue,
            "Alice",
            "Test data",
            releaseTime
        )).to.emit(twoPhaseDiss, "ReleaseEncryptedData").withArgs(bytesValue, "Alice", "Test data", releaseTime);
    });


   it("should emit KeyReleaseRequested event when time is ripe", async function () {
        const currentTime = await time.latest();
        const releaseTime = currentTime + 43200;

        const testData = {
            encryptedData: toUtf8Bytes("heia lyn"),
            owner: "Alice",
            dataName: "Test data",
            releaseTime: releaseTime
        };

        await twoPhaseDiss.addStoredData(
            testData.encryptedData,
            testData.owner,
            testData.dataName,
            testData.releaseTime
        );

        await time.increase(43200);

        const checkData = "0x";
        const {upkeepNeeded} = await twoPhaseDiss.checkUpkeep.staticCall(checkData);

        expect(upkeepNeeded).to.equal(true); 

        const tx = await twoPhaseDiss.performUpkeep(checkData);

        await expect(tx).to.emit(twoPhaseDiss, "KeyReleaseRequested").withArgs(0, testData.owner, testData.dataName);
    });


    it("Should revert if any of the strings are empty", async function () {
        const stringValue = "heia lyn";
        const bytesValue = toUtf8Bytes(stringValue);
        const releaseTime = (await time.latest()) + 60;

        await expect(twoPhaseDiss.addStoredData(
            bytesValue,
            "Alice",
            "",
            releaseTime
        )).to.be.revertedWith("Data name is required");

        await expect(twoPhaseDiss.addStoredData(
            bytesValue,
            "",
            "Test data",
            releaseTime
        )).to.be.revertedWith("Owner is required"); 
    });

    it("Should revert if releaseTime is in the past", async function () {
        const stringValue = "heia lyn";
        const bytesValue = toUtf8Bytes(stringValue);
        const releaseTime = (await time.latest()) - 60;

        await expect(twoPhaseDiss.addStoredData(
            bytesValue,
            "Alice",
            "Test data",
            releaseTime
        )).to.be.revertedWith("Release time must be in the future");
    });



});
